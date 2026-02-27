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
		"GET /api/v1/sessions/{id}/children", s.withTimeout(s.handleGetChildSessions),
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
	s.mux.Handle("GET /api/v1/agents", s.withTimeout(s.handleListAgents))
	s.mux.Handle("GET /api/v1/stats", s.withTimeout(s.handleGetStats))
	s.mux.Handle("GET /api/v1/version", s.withTimeout(s.handleGetVersion))
	s.mux.HandleFunc("POST /api/v1/sync", s.handleTriggerSync)
	s.mux.HandleFunc("POST /api/v1/resync", s.handleTriggerResync)
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
	allowedOrigins := buildAllowedOrigins(s.cfg.Host, s.cfg.Port)
	allowedHosts := buildAllowedHosts(s.cfg.Host, s.cfg.Port)
	bindAll := isBindAll(s.cfg.Host)
	return hostCheckMiddleware(allowedHosts, bindAll,
		corsMiddleware(allowedOrigins, bindAll, logMiddleware(s.mux)),
	)
}

// buildAllowedHosts returns the set of Host header values that
// are legitimate for this server. This defends against DNS
// rebinding attacks where an attacker's domain resolves to
// 127.0.0.1 — the browser sends the attacker's domain as the
// Host header, which we reject.
func buildAllowedHosts(host string, port int) map[string]bool {
	hosts := make(map[string]bool)
	add := func(h string) {
		hosts[net.JoinHostPort(h, strconv.Itoa(port))] = true
		// Browsers may omit port 80 from the Host header.
		// IPv6 literals need brackets (e.g., [::1]).
		if port == 80 {
			if strings.Contains(h, ":") {
				hosts["["+h+"]"] = true
			} else {
				hosts[h] = true
			}
		}
	}
	add(host)
	switch host {
	case "127.0.0.1":
		add("localhost")
	case "localhost":
		add("127.0.0.1")
	case "0.0.0.0", "::":
		add("127.0.0.1")
		add("localhost")
		add("::1")
	case "::1":
		add("127.0.0.1")
		add("localhost")
	}
	return hosts
}

// hostCheckMiddleware validates the Host header against expected
// values to prevent DNS rebinding attacks. Only applied to /api/
// routes — the SPA fallback is left accessible for flexibility.
// When bindAll is true (0.0.0.0/::), Host validation is skipped
// because LAN clients connect via the machine's real IP.
func hostCheckMiddleware(
	allowedHosts map[string]bool, bindAll bool, next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !bindAll {
			if !allowedHosts[r.Host] {
				http.Error(
					w, "Forbidden", http.StatusForbidden,
				)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// httpOrigin formats an HTTP origin string. It uses
// net.JoinHostPort to handle IPv6 bracket formatting correctly
// (e.g., [::1]:8080). Browsers omit the port from the Origin
// header for default ports (80 for HTTP), so for port 80 both
// forms are returned.
func httpOrigin(host string, port int) []string {
	hp := net.JoinHostPort(host, strconv.Itoa(port))
	origin := "http://" + hp
	if port == 80 {
		// net.JoinHostPort brackets IPv6, so use it for the
		// portless form too: JoinHostPort("::1","") is not
		// valid, so bracket manually when needed.
		bare := host
		if strings.Contains(host, ":") {
			bare = "[" + host + "]"
		}
		return []string{origin, "http://" + bare}
	}
	return []string{origin}
}

// buildAllowedOrigins returns the set of origins that should be
// permitted by CORS. For loopback addresses, both "127.0.0.1"
// and "localhost" are allowed because browsers treat them as
// distinct origins.
func buildAllowedOrigins(host string, port int) map[string]bool {
	origins := make(map[string]bool)
	add := func(h string) {
		for _, o := range httpOrigin(h, port) {
			origins[o] = true
		}
	}
	add(host)
	// When binding to a loopback address, also allow the other
	// loopback variants because browsers treat them as distinct
	// origins. When binding to 0.0.0.0 or :: (all interfaces),
	// allow all loopback origins since that's how browsers will
	// access a bind-all server.
	switch host {
	case "127.0.0.1":
		add("localhost")
	case "localhost":
		add("127.0.0.1")
	case "0.0.0.0", "::":
		add("127.0.0.1")
		add("localhost")
		add("::1")
	case "::1":
		add("127.0.0.1")
		add("localhost")
	}
	return origins
}

// isBindAll returns true when the server is listening on all
// interfaces (0.0.0.0 or ::), meaning LAN clients may connect
// via the machine's real IP or hostname.
func isBindAll(host string) bool {
	return host == "0.0.0.0" || host == "::"
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

// isMutating returns true for HTTP methods that change state.
func isMutating(method string) bool {
	return method == http.MethodPost ||
		method == http.MethodPut ||
		method == http.MethodPatch ||
		method == http.MethodDelete
}

func corsMiddleware(
	allowedOrigins map[string]bool, bindAll bool, next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			origin := r.Header.Get("Origin")
			// For reads (GET/HEAD), allow empty Origin (same-origin
			// requests often omit it). For mutating methods and
			// preflights, require Origin to be present and allowed.
			// When bindAll is true, any non-empty origin is
			// accepted because the user explicitly chose to expose
			// the server on the network.
			originAllowed := allowedOrigins[origin] ||
				(bindAll && origin != "")
			safeForReads := origin == "" || originAllowed

			if originAllowed {
				w.Header().Set(
					"Access-Control-Allow-Origin", origin,
				)
			}
			// Always set Vary so caches don't serve a
			// response without CORS headers to a
			// legitimate origin.
			w.Header().Set("Vary", "Origin")
			w.Header().Set(
				"Access-Control-Allow-Methods",
				"GET, POST, PUT, PATCH, DELETE, OPTIONS",
			)
			w.Header().Set(
				"Access-Control-Allow-Headers",
				"Content-Type",
			)
			if r.Method == http.MethodOptions {
				if !safeForReads {
					http.Error(
						w, "Forbidden", http.StatusForbidden,
					)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			// Block state-changing requests unless Origin
			// is present and recognized. This prevents
			// CSRF via simple requests (e.g., <form> POST)
			// and DNS rebinding where Origin is absent.
			if !originAllowed && isMutating(r.Method) {
				http.Error(
					w, "Forbidden", http.StatusForbidden,
				)
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
