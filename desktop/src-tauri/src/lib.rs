use std::collections::BTreeMap;
use std::error::Error;
use std::ffi::OsString;
use std::fs;
use std::io;
use std::io::{Read, Write};
use std::net::{Ipv4Addr, SocketAddrV4, TcpStream};
use std::path::PathBuf;
use std::process::Stdio;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{mpsc, Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant};

use tauri::async_runtime::Receiver;
use tauri::{App, AppHandle, Manager, RunEvent, WebviewWindow};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

const HOST: &str = "127.0.0.1";
const PREFERRED_PORT: u16 = 8080;
const READY_TIMEOUT: Duration = Duration::from_secs(30);
const READY_POLL_INTERVAL: Duration = Duration::from_millis(125);
const LOGIN_SHELL_ENV_TIMEOUT: Duration = Duration::from_secs(3);
const LOGIN_SHELL_READER_TIMEOUT: Duration = Duration::from_millis(300);

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
    let window = main_window(app)?;
    let (rx, child) = spawn_sidecar(app)?;

    save_sidecar(app, child)?;
    forward_sidecar_logs(rx, window);

    Ok(())
}

fn spawn_sidecar(app: &App) -> Result<(CommandRx, CommandChild), DynError> {
    let port_arg = PREFERRED_PORT.to_string();
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
    let skip_login_shell = std::env::var_os("AGENTSVIEW_DESKTOP_SKIP_LOGIN_SHELL_ENV");
    let should_probe =
        should_probe_login_shell(skip_login_shell.as_ref(), cfg!(target_os = "windows"));

    build_sidecar_env(
        std::env::vars_os().collect(),
        if should_probe {
            read_login_shell_env().unwrap_or_default()
        } else {
            Vec::new()
        },
        read_desktop_env_file(),
        std::env::var_os("AGENTSVIEW_DESKTOP_PATH"),
        cfg!(target_os = "windows"),
    )
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

    let stdout = run_login_shell_env(shell.as_str(), LOGIN_SHELL_ENV_TIMEOUT)?;
    Some(parse_nul_env(stdout.as_slice()))
}

// read_desktop_env_file parses ~/.agentsview/desktop.env as
// KEY=VALUE lines. This provides a manual override path before
// desktop settings UI exists.
fn read_desktop_env_file() -> Vec<(OsString, OsString)> {
    let Some(home) = resolve_home_dir() else {
        return Vec::new();
    };
    let path = home.join(".agentsview").join("desktop.env");
    let Ok(content) = fs::read_to_string(path) else {
        return Vec::new();
    };

    parse_desktop_env_content(content.as_str())
}

fn resolve_home_dir() -> Option<PathBuf> {
    resolve_home_dir_from_lookup(|key| std::env::var_os(key), cfg!(target_os = "windows"))
}

fn should_probe_login_shell(skip: Option<&OsString>, is_windows: bool) -> bool {
    !is_windows && skip.is_none()
}

fn build_sidecar_env(
    inherited: Vec<(OsString, OsString)>,
    login_shell: Vec<(OsString, OsString)>,
    desktop_file: Vec<(OsString, OsString)>,
    forced_path: Option<OsString>,
    case_insensitive_keys: bool,
) -> Vec<(OsString, OsString)> {
    let mut merged = BTreeMap::new();
    merge_env_pairs(&mut merged, inherited, case_insensitive_keys);
    merge_env_pairs(&mut merged, login_shell, case_insensitive_keys);
    merge_env_pairs(&mut merged, desktop_file, case_insensitive_keys);

    if let Some(path) = forced_path {
        merged.insert(
            normalize_env_key(std::ffi::OsStr::new("PATH"), case_insensitive_keys),
            path,
        );
    }

    merged.into_iter().collect()
}

fn merge_env_pairs(
    dest: &mut BTreeMap<OsString, OsString>,
    pairs: Vec<(OsString, OsString)>,
    case_insensitive_keys: bool,
) {
    for (k, v) in pairs {
        dest.insert(normalize_env_key(k.as_os_str(), case_insensitive_keys), v);
    }
}

fn normalize_env_key(key: &std::ffi::OsStr, case_insensitive_keys: bool) -> OsString {
    if case_insensitive_keys {
        return OsString::from(key.to_string_lossy().to_ascii_uppercase());
    }
    key.to_os_string()
}

fn run_login_shell_env(shell: &str, timeout: Duration) -> Option<Vec<u8>> {
    let mut child = std::process::Command::new(shell)
        .args(["-lic", "env -0"])
        .stdin(Stdio::null())
        .stderr(Stdio::null())
        .stdout(Stdio::piped())
        .spawn()
        .ok()?;
    let mut stdout = child.stdout.take()?;
    let (tx, rx) = mpsc::sync_channel(1);
    thread::spawn(move || {
        let mut out = Vec::new();
        let _ = tx.send(stdout.read_to_end(&mut out).ok().map(|_| out));
    });

    let deadline = Instant::now() + timeout;
    let status = loop {
        match child.try_wait().ok()? {
            Some(status) => {
                break status;
            }
            None => {
                if Instant::now() >= deadline {
                    let _ = child.kill();
                    let _ = child.wait();
                    let _ = rx.recv_timeout(LOGIN_SHELL_READER_TIMEOUT);
                    return None;
                }
                thread::sleep(Duration::from_millis(25));
            }
        }
    };
    if !status.success() {
        let _ = rx.recv_timeout(LOGIN_SHELL_READER_TIMEOUT);
        return None;
    }

    rx.recv_timeout(LOGIN_SHELL_READER_TIMEOUT).ok().flatten()
}

fn parse_nul_env(content: &[u8]) -> Vec<(OsString, OsString)> {
    let mut vars = Vec::new();
    for entry in content.split(|b| *b == 0) {
        if entry.is_empty() {
            continue;
        }
        let Some(eq) = entry.iter().position(|b| *b == b'=') else {
            continue;
        };
        if eq == 0 {
            continue;
        }
        vars.push((
            os_string_from_bytes(&entry[..eq]),
            os_string_from_bytes(&entry[eq + 1..]),
        ));
    }
    vars
}

#[cfg(unix)]
fn os_string_from_bytes(bytes: &[u8]) -> OsString {
    use std::os::unix::ffi::OsStringExt;
    OsString::from_vec(bytes.to_vec())
}

#[cfg(not(unix))]
fn os_string_from_bytes(bytes: &[u8]) -> OsString {
    OsString::from(String::from_utf8_lossy(bytes).into_owned())
}

fn parse_desktop_env_content(content: &str) -> Vec<(OsString, OsString)> {
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

fn resolve_home_dir_from_lookup<F>(mut lookup: F, prefer_userprofile: bool) -> Option<PathBuf>
where
    F: FnMut(&str) -> Option<OsString>,
{
    let get = |key: &str, lookup: &mut F| lookup(key).filter(|v| !v.is_empty());

    if prefer_userprofile {
        if let Some(profile) = get("USERPROFILE", &mut lookup) {
            return Some(PathBuf::from(profile));
        }
        if let Some(home) = get("HOME", &mut lookup) {
            return Some(PathBuf::from(home));
        }
    } else {
        if let Some(home) = get("HOME", &mut lookup) {
            return Some(PathBuf::from(home));
        }
        if let Some(profile) = get("USERPROFILE", &mut lookup) {
            return Some(PathBuf::from(profile));
        }
    }

    let drive = get("HOMEDRIVE", &mut lookup)?;
    let path = get("HOMEPATH", &mut lookup)?;
    let mut combined = OsString::from(drive);
    combined.push(path);
    Some(PathBuf::from(combined))
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

fn forward_sidecar_logs(mut rx: CommandRx, window: WebviewWindow) {
    let startup_handled = Arc::new(AtomicBool::new(false));
    let timeout_window = window.clone();
    let timeout_state = startup_handled.clone();
    thread::spawn(move || {
        thread::sleep(READY_TIMEOUT);
        if !timeout_state.load(Ordering::SeqCst) {
            let _ = timeout_window.eval(
                "document.getElementById('status').textContent = 'AgentsView backend did not become ready in time.';",
            );
        }
    });

    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(line_bytes) => {
                    let line = String::from_utf8_lossy(&line_bytes);
                    eprintln!("[agentsview] {}", line.trim_end());
                    if !startup_handled.load(Ordering::SeqCst) {
                        if let Some(port) = parse_listening_port(line.as_ref()) {
                            startup_handled.store(true, Ordering::SeqCst);
                            redirect_when_ready(window.clone(), port);
                        }
                    }
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
                    if !startup_handled.swap(true, Ordering::SeqCst) {
                        let _ = window.eval(
                            "document.getElementById('status').textContent = 'AgentsView backend exited before startup completed.';",
                        );
                    }
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

fn parse_listening_port(line: &str) -> Option<u16> {
    let marker = format!("listening at http://{HOST}:");
    let idx = line.find(marker.as_str())?;
    let after = &line[(idx + marker.len())..];
    let digits: String = after.chars().take_while(|ch| ch.is_ascii_digit()).collect();
    if digits.is_empty() {
        return None;
    }
    digits.parse::<u16>().ok()
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

fn wait_for_server(port: u16, timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        if backend_endpoint_ready(port) {
            return true;
        }
        thread::sleep(READY_POLL_INTERVAL);
    }
    false
}

fn backend_endpoint_ready(port: u16) -> bool {
    let request =
        format!("GET /api/v1/version HTTP/1.1\r\nHost: {HOST}:{port}\r\nConnection: close\r\n\r\n");
    let response = match read_http_response(port, request.as_str()) {
        Some(resp) => resp,
        None => return false,
    };
    version_response_looks_valid(response.as_slice())
}

fn read_http_response(port: u16, request: &str) -> Option<Vec<u8>> {
    let addr = SocketAddrV4::new(Ipv4Addr::LOCALHOST, port);
    let mut stream = match TcpStream::connect_timeout(&addr.into(), Duration::from_millis(250)) {
        Ok(stream) => stream,
        Err(_) => return None,
    };

    let _ = stream.set_read_timeout(Some(Duration::from_millis(250)));
    let _ = stream.set_write_timeout(Some(Duration::from_millis(250)));

    if stream.write_all(request.as_bytes()).is_err() {
        return None;
    }

    let mut buf = Vec::with_capacity(4096);
    if stream.read_to_end(&mut buf).is_err() {
        return None;
    }
    if buf.is_empty() {
        return None;
    }
    Some(buf)
}

fn version_response_looks_valid(response: &[u8]) -> bool {
    if !(response.starts_with(b"HTTP/1.1 200") || response.starts_with(b"HTTP/1.0 200")) {
        return false;
    }
    let body = if let Some(idx) = response.windows(4).position(|w| w == b"\r\n\r\n") {
        &response[(idx + 4)..]
    } else if let Some(idx) = response.windows(2).position(|w| w == b"\n\n") {
        &response[(idx + 2)..]
    } else {
        return false;
    };
    let body = String::from_utf8_lossy(body);
    body.contains("\"version\"") && body.contains("\"commit\"") && body.contains("\"build_date\"")
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    #[cfg(unix)]
    use std::os::unix::ffi::OsStrExt;
    #[cfg(unix)]
    use std::os::unix::fs::PermissionsExt;
    #[cfg(unix)]
    use std::time::{SystemTime, UNIX_EPOCH};

    #[test]
    fn parse_listening_port_extracts_backend_port() {
        let line = "agentsview dev listening at http://127.0.0.1:18080 (started in 1.2s)";
        assert_eq!(parse_listening_port(line), Some(18080));
        assert_eq!(parse_listening_port("unrelated line"), None);
    }

    #[test]
    fn parse_listening_port_ignores_non_listening_urls() {
        let line = "probe successful for http://127.0.0.1:19090/health";
        assert_eq!(parse_listening_port(line), None);
    }

    #[test]
    fn version_response_requires_identity_fields() {
        let valid = b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"version\":\"1.0.0\",\"commit\":\"abc\",\"build_date\":\"2026-01-01T00:00:00Z\"}";
        assert!(version_response_looks_valid(valid));

        let missing = b"HTTP/1.1 200 OK\r\n\r\n{\"version\":\"1.0.0\"}";
        assert!(!version_response_looks_valid(missing));

        let wrong_status = b"HTTP/1.1 404 Not Found\r\n\r\n{}";
        assert!(!version_response_looks_valid(wrong_status));
    }

    #[test]
    fn should_probe_login_shell_skips_windows_or_explicit_skip() {
        assert!(should_probe_login_shell(None, false));
        assert!(!should_probe_login_shell(Some(&OsString::from("1")), false));
        assert!(!should_probe_login_shell(None, true));
    }

    #[test]
    fn build_sidecar_env_applies_precedence_and_path_override() {
        let merged = build_sidecar_env(
            vec![
                (OsString::from("PATH"), OsString::from("/bin")),
                (OsString::from("HOME"), OsString::from("/base")),
            ],
            vec![(OsString::from("HOME"), OsString::from("/login"))],
            vec![(OsString::from("HOME"), OsString::from("/desktop"))],
            Some(OsString::from("/custom/path")),
            false,
        );
        let map: HashMap<_, _> = merged.into_iter().collect();
        assert_eq!(
            map.get(&OsString::from("HOME")),
            Some(&OsString::from("/desktop"))
        );
        assert_eq!(
            map.get(&OsString::from("PATH")),
            Some(&OsString::from("/custom/path"))
        );
    }

    #[test]
    fn build_sidecar_env_supports_case_insensitive_windows_keys() {
        let merged = build_sidecar_env(
            vec![(OsString::from("Path"), OsString::from("A"))],
            vec![(OsString::from("PATH"), OsString::from("B"))],
            vec![],
            Some(OsString::from("C")),
            true,
        );
        let map: HashMap<_, _> = merged.into_iter().collect();
        assert_eq!(map.len(), 1);
        assert_eq!(map.get(&OsString::from("PATH")), Some(&OsString::from("C")));
    }

    #[test]
    fn parse_desktop_env_content_ignores_comments_and_invalid_lines() {
        let parsed = parse_desktop_env_content(
            r#"
            # comment
            PATH=/custom/bin
            BADLINE
            =missingkey
            FOO = bar
            "#,
        );
        let map: HashMap<_, _> = parsed.into_iter().collect();
        assert_eq!(
            map.get(&OsString::from("PATH")),
            Some(&OsString::from("/custom/bin"))
        );
        assert_eq!(
            map.get(&OsString::from("FOO")),
            Some(&OsString::from("bar"))
        );
        assert!(!map.contains_key(&OsString::from("BADLINE")));
    }

    #[test]
    fn resolve_home_dir_from_lookup_honors_platform_precedence() {
        let mut lookup = HashMap::new();
        lookup.insert("HOME".to_string(), OsString::from("/home/a"));
        lookup.insert("USERPROFILE".to_string(), OsString::from("C:\\Users\\a"));
        let resolved_unix = resolve_home_dir_from_lookup(|k| lookup.get(k).cloned(), false);
        assert_eq!(resolved_unix, Some(PathBuf::from("/home/a")));

        let resolved_windows = resolve_home_dir_from_lookup(|k| lookup.get(k).cloned(), true);
        assert_eq!(resolved_windows, Some(PathBuf::from("C:\\Users\\a")));
    }

    #[test]
    fn parse_nul_env_tolerates_invalid_utf8_entries() {
        let raw = b"PATH=/bin\0BROKEN=\xFF\xFE\0EMPTY=\0\0";
        let parsed = parse_nul_env(raw);
        let map: HashMap<_, _> = parsed.into_iter().collect();
        assert!(map.contains_key(&OsString::from("PATH")));

        #[cfg(unix)]
        {
            let broken = map
                .get(&OsString::from("BROKEN"))
                .expect("BROKEN key present");
            assert_eq!(broken.as_os_str().as_bytes(), b"\xFF\xFE");
        }
    }

    #[cfg(unix)]
    #[test]
    fn run_login_shell_env_handles_large_stdout() {
        let stamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("valid clock")
            .as_nanos();
        let script_path = std::env::temp_dir().join(format!(
            "agentsview-login-shell-{stamp}-{}.sh",
            std::process::id()
        ));
        fs::write(&script_path, "#!/bin/sh\nhead -c 262144 /dev/zero\n")
            .expect("write shell script");
        let mut perms = fs::metadata(&script_path)
            .expect("read shell script metadata")
            .permissions();
        perms.set_mode(0o700);
        fs::set_permissions(&script_path, perms).expect("set executable permissions");

        let output = run_login_shell_env(
            script_path.to_str().expect("script path utf-8"),
            Duration::from_secs(2),
        );
        let _ = fs::remove_file(&script_path);

        let output = output.expect("expected shell output");
        assert!(
            output.len() >= 262_144,
            "expected at least 262144 bytes, got {}",
            output.len()
        );
    }

    #[cfg(unix)]
    #[test]
    fn run_login_shell_env_timeout_returns_when_stdout_fd_stays_open() {
        let stamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("valid clock")
            .as_nanos();
        let script_path = std::env::temp_dir().join(format!(
            "agentsview-login-shell-timeout-{stamp}-{}.sh",
            std::process::id()
        ));
        fs::write(&script_path, "#!/bin/sh\n(sleep 2) &\nsleep 10\n").expect("write shell script");
        let mut perms = fs::metadata(&script_path)
            .expect("read shell script metadata")
            .permissions();
        perms.set_mode(0o700);
        fs::set_permissions(&script_path, perms).expect("set executable permissions");

        let started = Instant::now();
        let output = run_login_shell_env(
            script_path.to_str().expect("script path utf-8"),
            Duration::from_millis(120),
        );
        let elapsed = started.elapsed();
        let _ = fs::remove_file(&script_path);

        assert!(output.is_none(), "timeout path should return None");
        assert!(
            elapsed < Duration::from_secs(1),
            "timeout path took too long: {elapsed:?}"
        );
    }
}
