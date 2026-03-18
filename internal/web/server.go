package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
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
	csrfToken string
	sendFunc  func(userID int64, text string) error
}

// New creates a new web server instance. Scheduler can be nil and set later via SetScheduler.
func New(cfg config.Config, store *storage.Storage, sched *scheduler.Scheduler) *Server {
	token := cfg.Web.AuthToken
	if token == "" {
		token = generateRandomToken()

		tokenFile := "data/web_token.txt"
		if err := os.MkdirAll("data", 0o700); err != nil {
			log.Printf("web: create data dir: %v", err)
		} else if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
			log.Printf("web: write token file: %v", err)
		}
		log.Printf("Web auth token generated, saved to %s", tokenFile)
	}

	s := &Server{
		cfg:       cfg,
		store:     store,
		scheduler: sched,
		mux:       http.NewServeMux(),
		authToken: token,
		csrfToken: generateRandomToken(),
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

// SetSendFunc sets the callback used to send Telegram messages from the admin panel.
func (s *Server) SetSendFunc(fn func(userID int64, text string) error) {
	s.sendFunc = fn
}

func (s *Server) registerRoutes() {
	// CSRF token endpoint — auth-protected, GET only
	s.mux.HandleFunc("/api/csrf", s.authAPI(s.handleCSRF))

	// API routes — protected by auth + CSRF middleware for state-changing methods
	s.mux.HandleFunc("/api/users", s.authAPI(s.handleUsers))
	s.mux.HandleFunc("/api/stats", s.authAPI(s.handleStats))
	s.mux.HandleFunc("/api/mood", s.authAPI(s.handleMood))
	s.mux.HandleFunc("/api/profile", s.authAPI(s.csrfCheck(s.handleProfile)))
	s.mux.HandleFunc("/api/achievements", s.authAPI(s.handleAchievements))
	s.mux.HandleFunc("/api/messages", s.authAPI(s.handleMessages))
	s.mux.HandleFunc("/api/config", s.authAPI(s.handleConfig))
	s.mux.HandleFunc("/api/config/scheduler", s.authAPI(s.csrfCheck(s.handleConfigScheduler)))
	s.mux.HandleFunc("/api/config/hours", s.authAPI(s.csrfCheck(s.handleConfigHours)))
	s.mux.HandleFunc("/api/summary", s.authAPI(s.handleSummary))
	s.mux.HandleFunc("/api/send", s.authAPI(s.csrfCheck(s.handleSend)))
	s.mux.HandleFunc("/api/scheduler/ping", s.authAPI(s.csrfCheck(s.handleSchedulerPing)))
	s.mux.HandleFunc("/api/backup", s.authAPI(s.handleBackup))
	s.mux.HandleFunc("/api/prompts", s.authAPI(s.csrfCheck(s.handlePrompts)))
	s.mux.HandleFunc("/api/easter-eggs", s.authAPI(s.csrfCheck(s.handleEasterEggs)))
	s.mux.HandleFunc("/api/analytics", s.authAPI(s.handleAnalytics))
	s.mux.HandleFunc("/api/custom-achievements", s.authAPI(s.csrfCheck(s.handleCustomAchievements)))
	s.mux.HandleFunc("/api/runtime-config", s.authAPI(s.csrfCheck(s.handleRuntimeConfig)))

	// Static files — protected by auth middleware (cookie or query param)
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("web: embed static: %v", err)
	}
	s.mux.Handle("/", s.authStatic(http.FileServer(http.FS(staticFS))))
}

// csrfCheck validates the X-CSRF-Token header on POST and DELETE requests.
func (s *Server) csrfCheck(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodDelete {
			token := r.Header.Get("X-CSRF-Token")
			if token != s.csrfToken {
				s.writeError(w, http.StatusForbidden, "invalid csrf token")
				return
			}
		}
		next(w, r)
	}
}

// handleCSRF returns the CSRF token for use in state-changing requests.
func (s *Server) handleCSRF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.writeJSON(w, map[string]string{"token": s.csrfToken})
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
	addr := fmt.Sprintf("127.0.0.1:%d", s.cfg.Web.Port)
	log.Printf("Web dashboard started on http://%s", addr)

	if err := http.ListenAndServe(addr, s.mux); err != nil {
		log.Printf("web server error: %v", err)
	}
}
