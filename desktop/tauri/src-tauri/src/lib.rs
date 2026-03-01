use std::collections::BTreeMap;
use std::error::Error;
use std::ffi::OsString;
use std::fs;
use std::io;
use std::io::{Read, Write};
use std::net::{Ipv4Addr, SocketAddrV4, TcpListener, TcpStream};
use std::path::PathBuf;
use std::sync::Mutex;
use std::thread;
use std::time::{Duration, Instant};

use tauri::async_runtime::Receiver;
use tauri::{App, AppHandle, Manager, RunEvent, WebviewWindow};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

const HOST: &str = "127.0.0.1";
const READY_TIMEOUT: Duration = Duration::from_secs(30);
const READY_POLL_INTERVAL: Duration = Duration::from_millis(125);

type DynError = Box<dyn Error>;
type CommandRx = Receiver<CommandEvent>;

#[derive(Default)]
struct SidecarState {
    child: Mutex<Option<CommandChild>>,
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .manage(SidecarState::default())
        .setup(launch_backend)
        .build(tauri::generate_context!())
        .expect("failed to build tauri app")
        .run(|app_handle, event| {
            if matches!(event, RunEvent::ExitRequested { .. } | RunEvent::Exit) {
                stop_backend(app_handle);
            }
        });
}

fn launch_backend(app: &mut App) -> Result<(), DynError> {
    let port = reserve_port()?;
    let (rx, child) = spawn_sidecar(app, port)?;

    save_sidecar(app, child)?;
    forward_sidecar_logs(rx);
    redirect_when_ready(main_window(app)?, port);

    Ok(())
}

fn spawn_sidecar(app: &App, port: u16) -> Result<(CommandRx, CommandChild), DynError> {
    let port_arg = port.to_string();
    let mut command = app.shell().sidecar("agentsview")?;
    for (key, value) in sidecar_env() {
        command = command.env(key, value);
    }

    Ok(command
        .args([
            "serve",
            "-no-browser",
            "-host",
            HOST,
            "-port",
            port_arg.as_str(),
        ])
        .spawn()?)
}

// sidecar_env returns the environment passed to the backend
// sidecar process. It merges the app environment with
// login-shell variables so desktop launches inherit zshrc/bash
// exports. An optional ~/.agentsview/desktop.env file can
// override specific keys as an escape hatch.
fn sidecar_env() -> Vec<(OsString, OsString)> {
    let mut merged: BTreeMap<OsString, OsString> = std::env::vars_os().collect();

    if std::env::var_os("AGENTSVIEW_DESKTOP_SKIP_LOGIN_SHELL_ENV").is_none() {
        if let Some(login_shell_env) = read_login_shell_env() {
            for (k, v) in login_shell_env {
                merged.insert(k, v);
            }
        }
    }

    for (k, v) in read_desktop_env_file() {
        merged.insert(k, v);
    }

    if let Some(path) = std::env::var_os("AGENTSVIEW_DESKTOP_PATH") {
        merged.insert(OsString::from("PATH"), path);
    }

    merged.into_iter().collect()
}

// read_login_shell_env invokes the user's login shell and
// parses NUL-delimited env output (`env -0`).
fn read_login_shell_env() -> Option<Vec<(OsString, OsString)>> {
    let default_shell = if cfg!(target_os = "macos") {
        "/bin/zsh"
    } else {
        "/bin/sh"
    };
    let shell = std::env::var("SHELL")
        .ok()
        .filter(|s| !s.trim().is_empty())
        .unwrap_or_else(|| default_shell.to_string());

    let output = std::process::Command::new(shell)
        .args(["-lic", "env -0"])
        .output()
        .ok()?;
    if !output.status.success() {
        return None;
    }

    let content = String::from_utf8(output.stdout).ok()?;
    let mut vars = Vec::new();
    for entry in content.split('\0') {
        if entry.is_empty() {
            continue;
        }
        if let Some((k, v)) = entry.split_once('=') {
            vars.push((OsString::from(k), OsString::from(v)));
        }
    }
    Some(vars)
}

// read_desktop_env_file parses ~/.agentsview/desktop.env as
// KEY=VALUE lines. This provides a manual override path before
// desktop settings UI exists.
fn read_desktop_env_file() -> Vec<(OsString, OsString)> {
    let Some(home) = std::env::var_os("HOME") else {
        return Vec::new();
    };
    let path = PathBuf::from(home).join(".agentsview").join("desktop.env");
    let Ok(content) = fs::read_to_string(path) else {
        return Vec::new();
    };

    let mut vars = Vec::new();
    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let Some((k, v)) = line.split_once('=') else {
            continue;
        };
        let key = k.trim();
        if key.is_empty() {
            continue;
        }
        vars.push((OsString::from(key), OsString::from(v.trim())));
    }
    vars
}

fn save_sidecar(app: &App, child: CommandChild) -> Result<(), DynError> {
    let state = app.state::<SidecarState>();
    let mut guard = state
        .child
        .lock()
        .map_err(|_| io::Error::other("sidecar state lock poisoned"))?;
    *guard = Some(child);
    Ok(())
}

fn forward_sidecar_logs(mut rx: CommandRx) {
    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(line_bytes) => {
                    let line = String::from_utf8_lossy(&line_bytes);
                    eprintln!("[agentsview] {}", line.trim_end());
                }
                CommandEvent::Stderr(line_bytes) => {
                    let line = String::from_utf8_lossy(&line_bytes);
                    eprintln!("[agentsview:stderr] {}", line.trim_end());
                }
                CommandEvent::Terminated(payload) => {
                    eprintln!(
                        "[agentsview] sidecar terminated (code: {:?}, signal: {:?})",
                        payload.code, payload.signal
                    );
                    break;
                }
                CommandEvent::Error(err) => {
                    eprintln!("[agentsview:error] {err}");
                }
                _ => {}
            }
        }
    });
}

fn main_window(app: &App) -> Result<WebviewWindow, DynError> {
    app.get_webview_window("main")
        .ok_or_else(|| io::Error::other("missing main window").into())
}

fn redirect_when_ready(window: WebviewWindow, port: u16) {
    let target_url = format!("http://{HOST}:{port}");

    thread::spawn(move || {
        if wait_for_server(port, READY_TIMEOUT) {
            let script = format!("window.location.replace({target_url:?});");
            let _ = window.eval(&script);
            return;
        }

        let _ = window.eval(
            "document.getElementById('status').textContent = 'AgentsView backend did not start within 30 seconds.';",
        );
    });
}

fn stop_backend(app: &AppHandle) {
    let state = app.state::<SidecarState>();
    let Ok(mut guard) = state.child.lock() else {
        return;
    };

    if let Some(child) = guard.take() {
        if let Err(err) = child.kill() {
            eprintln!("[agentsview] failed to stop sidecar: {err}");
        }
    }
}

fn reserve_port() -> Result<u16, DynError> {
    let listener = TcpListener::bind((HOST, 0))?;
    Ok(listener.local_addr()?.port())
}

fn wait_for_server(port: u16, timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        if stats_endpoint_ready(port) {
            return true;
        }
        thread::sleep(READY_POLL_INTERVAL);
    }
    false
}

fn stats_endpoint_ready(port: u16) -> bool {
    let addr = SocketAddrV4::new(Ipv4Addr::LOCALHOST, port);
    let mut stream = match TcpStream::connect_timeout(&addr.into(), Duration::from_millis(250)) {
        Ok(stream) => stream,
        Err(_) => return false,
    };

    let _ = stream.set_read_timeout(Some(Duration::from_millis(250)));
    let _ = stream.set_write_timeout(Some(Duration::from_millis(250)));

    let request =
        format!("GET /api/v1/stats HTTP/1.1\r\nHost: {HOST}:{port}\r\nConnection: close\r\n\r\n");

    if stream.write_all(request.as_bytes()).is_err() {
        return false;
    }

    let mut buf = [0_u8; 256];
    let n = match stream.read(&mut buf) {
        Ok(n) => n,
        Err(_) => return false,
    };
    if n == 0 {
        return false;
    }

    let header = String::from_utf8_lossy(&buf[..n]);
    header.starts_with("HTTP/1.1 200") || header.starts_with("HTTP/1.0 200")
}
