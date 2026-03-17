package web

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/Arkosh744/chaos-bro-bot/internal/config"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

//go:embed static
var staticFiles embed.FS

// Server serves the web dashboard and API endpoints.
type Server struct {
	cfg   config.Config
	store *storage.Storage
	mux   *http.ServeMux
}

// New creates a new web server instance.
func New(cfg config.Config, store *storage.Storage) *Server {
	s := &Server{
		cfg:   cfg,
		store: store,
		mux:   http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// API routes
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/api/mood", s.handleMood)
	s.mux.HandleFunc("/api/profile", s.handleProfile)
	s.mux.HandleFunc("/api/achievements", s.handleAchievements)
	s.mux.HandleFunc("/api/messages", s.handleMessages)
	s.mux.HandleFunc("/api/config", s.handleConfig)

	// Static files — serve embedded FS
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("web: embed static: %v", err)
	}
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
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
