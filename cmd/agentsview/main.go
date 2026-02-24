package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
	_ "time/tzdata"

	"github.com/wesm/agentsview/internal/config"
	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/server"
	"github.com/wesm/agentsview/internal/sync"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = ""
)

const (
	periodicSyncInterval = 15 * time.Minute
	watcherDebounce      = 500 * time.Millisecond
	browserPollInterval  = 100 * time.Millisecond
	browserPollAttempts  = 60
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "prune":
			runPrune(os.Args[2:])
			return
		case "update":
			runUpdate(os.Args[2:])
			return
		case "serve":
			runServe(os.Args[2:])
			return
		case "version", "--version", "-v":
			fmt.Printf("agentsview %s (commit %s, built %s)\n",
				version, commit, buildDate)
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	runServe(os.Args[1:])
}

func printUsage() {
	fmt.Printf(`agentsview %s - local web viewer for AI agent sessions

Syncs Claude Code, Codex, and Gemini CLI session data into SQLite,
serves an analytics dashboard and session browser via a local web UI.

Usage:
  agentsview [flags]          Start the server (default command)
  agentsview serve [flags]    Start the server (explicit)
  agentsview prune [flags]    Delete sessions matching filters
  agentsview update [flags]   Check for and install updates
  agentsview version          Show version information
  agentsview help             Show this help

Server flags:
  -host string        Host to bind to (default "127.0.0.1")
  -port int           Port to listen on (default 8080)
  -no-browser         Don't open browser on startup

Prune flags:
  -project string     Sessions whose project contains this substring
  -max-messages int   Sessions with at most N messages (default -1)
  -before string      Sessions that ended before this date (YYYY-MM-DD)
  -first-message str  Sessions whose first message starts with this text
  -dry-run            Show what would be pruned without deleting
  -yes                Skip confirmation prompt

Update flags:
  -check              Check for updates without installing
  -yes                Install without confirmation prompt
  -force              Force check (ignore cache)

Environment variables:
  CLAUDE_PROJECTS_DIR     Claude Code projects directory
  CODEX_SESSIONS_DIR      Codex sessions directory
  GEMINI_DIR              Gemini CLI directory
  AGENT_VIEWER_DATA_DIR   Data directory (database, config)

Data is stored in ~/.agentsview/ by default.
`, version)
}

func runServe(args []string) {
	cfg := mustLoadConfig(args)
	database := mustOpenDB(cfg)
	defer database.Close()

	engine := sync.NewEngine(
		database, cfg.ClaudeProjectDir,
		cfg.CodexSessionsDir, cfg.GeminiDir, "local",
	)

	runInitialSync(engine)

	stopWatcher := startFileWatcher(cfg, engine)
	defer stopWatcher()

	go startPeriodicSync(engine)

	port := server.FindAvailablePort(cfg.Host, cfg.Port)
	if port != cfg.Port {
		fmt.Printf("Port %d in use, using %d\n", cfg.Port, port)
	}
	cfg.Port = port

	srv := server.New(cfg, database, engine,
		server.WithVersion(server.VersionInfo{
			Version:   version,
			Commit:    commit,
			BuildDate: buildDate,
		}),
	)

	url := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	fmt.Printf("agentsview %s listening at %s\n", version, url)

	if !cfg.NoBrowser {
		go openBrowser(url)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func mustLoadConfig(args []string) config.Config {
	fs := flag.NewFlagSet("agentsview", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(),
			"Usage: agentsview [serve] [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	config.RegisterServeFlags(fs)
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parsing flags: %v", err)
	}

	cfg, err := config.Load(fs)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("creating data dir: %v", err)
	}
	return cfg
}

func mustOpenDB(cfg config.Config) *db.DB {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}

	if cfg.CursorSecret != "" {
		secret, err := base64.StdEncoding.DecodeString(cfg.CursorSecret)
		if err != nil {
			log.Fatalf("invalid cursor secret: %v", err)
		}
		database.SetCursorSecret(secret)
	}

	return database
}

func runInitialSync(engine *sync.Engine) {
	fmt.Println("Running initial sync...")
	stats := engine.SyncAll(printSyncProgress)
	fmt.Printf(
		"\nSync complete: %d sessions (%d synced, %d skipped)\n",
		stats.TotalSessions, stats.Synced, stats.Skipped,
	)
}

func printSyncProgress(p sync.Progress) {
	if p.SessionsTotal > 0 {
		fmt.Printf(
			"\r  %d/%d sessions (%.0f%%) · %d messages",
			p.SessionsDone, p.SessionsTotal,
			p.Percent(), p.MessagesIndexed,
		)
	}
}

func startFileWatcher(
	cfg config.Config, engine *sync.Engine,
) func() {
	onChange := func(paths []string) {
		engine.SyncPaths(paths)
	}
	watcher, err := sync.NewWatcher(watcherDebounce, onChange)
	if err != nil {
		log.Printf("warning: file watcher unavailable: %v", err)
		return func() {}
	}

	if _, err := os.Stat(cfg.ClaudeProjectDir); err == nil {
		_ = watcher.WatchRecursive(cfg.ClaudeProjectDir)
	}
	if _, err := os.Stat(cfg.CodexSessionsDir); err == nil {
		_ = watcher.WatchRecursive(cfg.CodexSessionsDir)
	}
	if cfg.GeminiDir != "" {
		geminiTmp := filepath.Join(cfg.GeminiDir, "tmp")
		if _, err := os.Stat(geminiTmp); err == nil {
			_ = watcher.WatchRecursive(geminiTmp)
		}
	}
	watcher.Start()
	return watcher.Stop
}

func startPeriodicSync(engine *sync.Engine) {
	ticker := time.NewTicker(periodicSyncInterval)
	defer ticker.Stop()
	for range ticker.C {
		log.Println("Running scheduled sync...")
		engine.SyncAll(nil)
	}
}

func openBrowser(url string) {
	for range browserPollAttempts {
		time.Sleep(browserPollInterval)
		resp, err := http.Get(url + "/api/v1/stats")
		if err == nil {
			resp.Body.Close()
			break
		}
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32",
			"url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Run()
}
