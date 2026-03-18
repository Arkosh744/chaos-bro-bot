package web

import (
	"net/http"

	"github.com/Arkosh744/chaos-bro-bot/internal/metrics"
)

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	metrics.WriteTo(w)
}
