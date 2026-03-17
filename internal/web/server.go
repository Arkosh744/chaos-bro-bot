package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/config"
	"github.com/Arkosh744/chaos-bro-bot/internal/scheduler"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

//go:embed static
var staticFiles embed.FS

// Server serves the web dashboard and API endpoints.
type Server struct {
	cfg       config.Config
	store     *storage.Storage
	scheduler *scheduler.Scheduler
	mux       *http.ServeMux
	authToken string
}

// New creates a new web server instance. Scheduler can be nil and set later via SetScheduler.
func New(cfg config.Config, store *storage.Storage, sched *scheduler.Scheduler) *Server {
	token := cfg.Web.AuthToken
	if token == "" {
		token = generateRandomToken()
		log.Printf("Web auth token (generated): %s", token)
	}

	s := &Server{
		cfg:       cfg,
		store:     store,
		scheduler: sched,
		mux:       http.NewServeMux(),
		authToken: token,
	}
	s.registerRoutes()
	return s
}

// generateRandomToken creates a random 32-char hex string.
func generateRandomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("web: generate auth token: %v", err)
	}
	return hex.EncodeToString(b)
}

// SetScheduler sets the scheduler reference (called after bot init).
func (s *Server) SetScheduler(sched *scheduler.Scheduler) {
	s.scheduler = sched
}

func (s *Server) registerRoutes() {
	// API routes — protected by auth middleware
	s.mux.HandleFunc("/api/users", s.authAPI(s.handleUsers))
	s.mux.HandleFunc("/api/stats", s.authAPI(s.handleStats))
	s.mux.HandleFunc("/api/mood", s.authAPI(s.handleMood))
	s.mux.HandleFunc("/api/profile", s.authAPI(s.handleProfile))
	s.mux.HandleFunc("/api/achievements", s.authAPI(s.handleAchievements))
	s.mux.HandleFunc("/api/messages", s.authAPI(s.handleMessages))
	s.mux.HandleFunc("/api/config", s.authAPI(s.handleConfig))
	s.mux.HandleFunc("/api/config/scheduler", s.authAPI(s.handleConfigScheduler))
	s.mux.HandleFunc("/api/config/hours", s.authAPI(s.handleConfigHours))

	// Static files — protected by auth middleware (cookie or query param)
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("web: embed static: %v", err)
	}
	s.mux.Handle("/", s.authStatic(http.FileServer(http.FS(staticFS))))
}

// authAPI wraps an API handler with Bearer token authentication.
func (s *Server) authAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		if s.checkBearerToken(r) || s.checkQueryToken(r) || s.checkCookieToken(r) {
			next(w, r)
			return
		}

		s.writeError(w, http.StatusUnauthorized, "unauthorized")
	}
}

// authStatic wraps a static file handler with cookie/query param authentication.
func (s *Server) authStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.checkBearerToken(r) || s.checkCookieToken(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Check query param and set cookie on success for browser access
		if s.checkQueryToken(r) {
			http.SetCookie(w, &http.Cookie{
				Name:     "auth_token",
				Value:    s.authToken,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func (s *Server) checkBearerToken(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	return strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.authToken
}

func (s *Server) checkQueryToken(r *http.Request) bool {
	return r.URL.Query().Get("token") == s.authToken
}

func (s *Server) checkCookieToken(r *http.Request) bool {
	cookie, err := r.Cookie("auth_token")
	return err == nil && cookie.Value == s.authToken
}

// Start launches the HTTP server in the current goroutine.
// Typically called via `go server.Start()`.
func (s *Server) Start() {
	addr := fmt.Sprintf(":%d", s.cfg.Web.Port)
	log.Printf("Web dashboard started on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, s.mux); err != nil {
		log.Printf("web server error: %v", err)
	}
}
