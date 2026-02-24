package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/wesm/agentsview/internal/config"
	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/insight"
	"github.com/wesm/agentsview/internal/sync"
	"github.com/wesm/agentsview/internal/web"
)

// VersionInfo holds build-time version metadata.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

// Server is the HTTP server that serves the SPA and REST API.
type Server struct {
	mu      gosync.RWMutex
	cfg     config.Config
	db      *db.DB
	engine  *sync.Engine
	mux     *http.ServeMux
	httpSrv *http.Server
	version VersionInfo

	generateFunc insight.GenerateFunc
	spaFS        fs.FS
	spaHandler   http.Handler

	// handlerDelay is injected before each timeout-wrapped
	// handler, used only by tests to guarantee handlers
	// exceed a short timeout. Zero in production.
	handlerDelay time.Duration
}

// New creates a new Server.
func New(
	cfg config.Config, database *db.DB, engine *sync.Engine,
	opts ...Option,
) *Server {
	dist, err := web.Assets()
	if err != nil {
		log.Fatalf("embedded frontend not found: %v", err)
	}

	s := &Server{
		cfg:          cfg,
		db:           database,
		engine:       engine,
		mux:          http.NewServeMux(),
		generateFunc: insight.Generate,
		spaFS:        dist,
		spaHandler:   http.FileServerFS(dist),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.routes()
	return s
}

// Option configures a Server.
type Option func(*Server)

// WithVersion sets the build-time version metadata.
func WithVersion(v VersionInfo) Option {
	return func(s *Server) { s.version = v }
}

// WithGenerateFunc overrides the insight generation function,
// allowing tests to substitute a stub. Nil is ignored.
func WithGenerateFunc(f insight.GenerateFunc) Option {
	return func(s *Server) {
		if f != nil {
			s.generateFunc = f
		}
	}
}

func (s *Server) routes() {
	// API v1 routes
	s.mux.Handle("GET /api/v1/sessions", s.withTimeout(s.handleListSessions))
	s.mux.Handle("GET /api/v1/sessions/{id}", s.withTimeout(s.handleGetSession))
	s.mux.Handle(
		"GET /api/v1/sessions/{id}/messages", s.withTimeout(s.handleGetMessages),
	)
	s.mux.Handle(
		"GET /api/v1/sessions/{id}/minimap", s.withTimeout(s.handleGetMinimap),
	)
	// SSE: Do not use timeout, as this is a long-lived connection.
	s.mux.HandleFunc(
		"GET /api/v1/sessions/{id}/watch", s.handleWatchSession,
	)
	// Export: Do not use timeout handler to support large downloads and avoid buffering.
	s.mux.Handle(
		"GET /api/v1/sessions/{id}/export", http.HandlerFunc(s.handleExportSession),
	)
	s.mux.Handle(
		"POST /api/v1/sessions/{id}/publish", s.withTimeout(s.handlePublishSession),
	)
	s.mux.Handle(
		"POST /api/v1/sessions/upload", s.withTimeout(s.handleUploadSession),
	)
	s.mux.Handle("GET /api/v1/analytics/summary", s.withTimeout(s.handleAnalyticsSummary))
	s.mux.Handle("GET /api/v1/analytics/activity", s.withTimeout(s.handleAnalyticsActivity))
	s.mux.Handle("GET /api/v1/analytics/heatmap", s.withTimeout(s.handleAnalyticsHeatmap))
	s.mux.Handle("GET /api/v1/analytics/projects", s.withTimeout(s.handleAnalyticsProjects))
	s.mux.Handle("GET /api/v1/analytics/hour-of-week", s.withTimeout(s.handleAnalyticsHourOfWeek))
	s.mux.Handle("GET /api/v1/analytics/sessions", s.withTimeout(s.handleAnalyticsSessionShape))
	s.mux.Handle("GET /api/v1/analytics/velocity", s.withTimeout(s.handleAnalyticsVelocity))
	s.mux.Handle("GET /api/v1/analytics/tools", s.withTimeout(s.handleAnalyticsTools))
	s.mux.Handle("GET /api/v1/analytics/top-sessions", s.withTimeout(s.handleAnalyticsTopSessions))

	s.mux.Handle("GET /api/v1/insights", s.withTimeout(s.handleListInsights))
	s.mux.Handle("GET /api/v1/insights/{id}", s.withTimeout(s.handleGetInsight))
	s.mux.Handle("DELETE /api/v1/insights/{id}", s.withTimeout(s.handleDeleteInsight))
	s.mux.HandleFunc("POST /api/v1/insights/generate", s.handleGenerateInsight)

	s.mux.Handle("GET /api/v1/search", s.withTimeout(s.handleSearch))
	s.mux.Handle("GET /api/v1/projects", s.withTimeout(s.handleListProjects))
	s.mux.Handle("GET /api/v1/machines", s.withTimeout(s.handleListMachines))
	s.mux.Handle("GET /api/v1/stats", s.withTimeout(s.handleGetStats))
	s.mux.Handle("GET /api/v1/version", s.withTimeout(s.handleGetVersion))
	s.mux.HandleFunc("POST /api/v1/sync", s.handleTriggerSync)
	s.mux.Handle("GET /api/v1/sync/status", s.withTimeout(s.handleSyncStatus))
	s.mux.Handle("GET /api/v1/config/github", s.withTimeout(s.handleGetGithubConfig))
	s.mux.Handle(
		"POST /api/v1/config/github", s.withTimeout(s.handleSetGithubConfig),
	)

	// SPA fallback: serve embedded frontend
	// Do not use timeout handler for static assets to avoid buffering.
	s.mux.Handle("/", http.HandlerFunc(s.handleSPA))
}

func (s *Server) handleGetVersion(
	w http.ResponseWriter, _ *http.Request,
) {
	writeJSON(w, http.StatusOK, s.version)
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	// Try to serve the exact file
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	f, err := s.spaFS.Open(path)
	if err == nil {
		f.Close()
		s.spaHandler.ServeHTTP(w, r)
		return
	}

	// SPA fallback: serve index.html for all routes
	r.URL.Path = "/"
	s.spaHandler.ServeHTTP(w, r)
}

// SetPort updates the listen port (for testing).
func (s *Server) SetPort(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Port = port
}

// SetGithubToken updates the GitHub token for testing.
func (s *Server) SetGithubToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.GithubToken = token
}

// githubToken returns the current GitHub token (thread-safe).
func (s *Server) githubToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.GithubToken
}

// Handler returns the http.Handler with middleware applied.
func (s *Server) Handler() http.Handler {
	return corsMiddleware(logMiddleware(s.mux))
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	srv := &http.Server{
		Addr:        addr,
		Handler:     s.Handler(),
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 120 * time.Second,
	}
	s.mu.Lock()
	s.httpSrv = srv
	s.mu.Unlock()
	log.Printf("Starting server at http://%s", addr)
	return srv.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.RLock()
	srv := s.httpSrv
	s.mu.RUnlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// FindAvailablePort finds an available port starting from the
// given port, binding to the specified host.
func FindAvailablePort(host string, start int) int {
	for port := start; port < start+100; port++ {
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return port
		}
	}
	return start
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set(
				"Access-Control-Allow-Origin", "*",
			)
			w.Header().Set(
				"Access-Control-Allow-Methods",
				"GET, POST, DELETE, OPTIONS",
			)
			w.Header().Set(
				"Access-Control-Allow-Headers",
				"Content-Type",
			)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
