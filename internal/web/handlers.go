package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
)

func (s *Server) ownerID() int64 {
	return s.cfg.Telegram.OwnerID
}

func (s *Server) writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("web: json encode: %v", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// handleStats returns message counts and last activity time.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	uid := s.ownerID()

	total, err := s.store.GetMessageCount(uid)
	if err != nil {
		log.Printf("web: get message count: %v", err)
	}

	today, err := s.store.GetMessageCountToday(uid)
	if err != nil {
		log.Printf("web: get message count today: %v", err)
	}

	weekAgo := time.Now().AddDate(0, 0, -7)
	week, err := s.store.GetMessageCountSinceDate(uid, weekAgo)
	if err != nil {
		log.Printf("web: get message count week: %v", err)
	}

	lastActivity, err := s.store.LastMessageTime(uid)
	if err != nil {
		log.Printf("web: last message time: %v", err)
	}

	var lastActivityStr string
	if !lastActivity.IsZero() {
		lastActivityStr = lastActivity.Format("2006-01-02 15:04:05")
	}

	hourly, err := s.store.GetHourlyActivity(uid)
	if err != nil {
		log.Printf("web: get hourly activity: %v", err)
	}

	// Build a 24-element array for heatmap
	heatmap := make([]int, 24)
	for _, h := range hourly {
		if h.Hour >= 0 && h.Hour < 24 {
			heatmap[h.Hour] = h.Count
		}
	}

	s.writeJSON(w, map[string]any{
		"total":         total,
		"today":         today,
		"week":          week,
		"last_activity": lastActivityStr,
		"heatmap":       heatmap,
	})
}

// handleMood returns mood history over the last N days (default 30).
func (s *Server) handleMood(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	entries, err := s.store.GetMoodHistory(s.ownerID(), days)
	if err != nil {
		log.Printf("web: get mood history: %v", err)
		s.writeJSON(w, []any{})
		return
	}

	type moodPoint struct {
		Score int    `json:"score"`
		Date  string `json:"date"`
	}
	result := make([]moodPoint, 0, len(entries))
	for _, e := range entries {
		result = append(result, moodPoint{
			Score: e.Score,
			Date:  e.CreatedAt.Format("2006-01-02"),
		})
	}

	s.writeJSON(w, result)
}

// handleProfile handles GET (list facts) and POST (update a fact).
func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	uid := s.ownerID()

	switch r.Method {
	case http.MethodGet:
		facts, err := s.store.GetFacts(uid)
		if err != nil {
			log.Printf("web: get facts: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to get facts")
			return
		}

		type factDTO struct {
			Category  string `json:"category"`
			Fact      string `json:"fact"`
			UpdatedAt string `json:"updated_at"`
		}
		result := make([]factDTO, 0, len(facts))
		for _, f := range facts {
			result = append(result, factDTO{
				Category:  f.Category,
				Fact:      f.Fact,
				UpdatedAt: f.UpdatedAt.Format("2006-01-02 15:04"),
			})
		}
		s.writeJSON(w, result)

	case http.MethodPost:
		var req struct {
			Category string `json:"category"`
			Fact     string `json:"fact"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Category == "" {
			s.writeError(w, http.StatusBadRequest, "category is required")
			return
		}

		// Empty fact means delete
		if req.Fact == "" {
			if err := s.store.DeleteFact(uid, req.Category); err != nil {
				log.Printf("web: delete fact: %v", err)
				s.writeError(w, http.StatusInternalServerError, "failed to delete fact")
				return
			}
		} else {
			if err := s.store.SaveFact(uid, req.Category, req.Fact); err != nil {
				log.Printf("web: save fact: %v", err)
				s.writeError(w, http.StatusInternalServerError, "failed to save fact")
				return
			}
		}

		s.writeJSON(w, map[string]string{"status": "ok"})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAchievements returns all achievements with their unlock status.
func (s *Server) handleAchievements(w http.ResponseWriter, r *http.Request) {
	uid := s.ownerID()

	unlocked, err := s.store.GetAchievements(uid)
	if err != nil {
		log.Printf("web: get achievements: %v", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get achievements")
		return
	}

	unlockedSet := make(map[string]bool, len(unlocked))
	for _, name := range unlocked {
		unlockedSet[name] = true
	}

	type achDTO struct {
		Key      string `json:"key"`
		Name     string `json:"name"`
		Emoji    string `json:"emoji"`
		Desc     string `json:"desc"`
		Unlocked bool   `json:"unlocked"`
	}

	result := make([]achDTO, 0, len(features.Achievements))
	for key, def := range features.Achievements {
		result = append(result, achDTO{
			Key:      key,
			Name:     def.Name,
			Emoji:    def.Emoji,
			Desc:     def.Desc,
			Unlocked: unlockedSet[key],
		})
	}

	s.writeJSON(w, result)
}

// handleMessages returns the last N messages (default 50).
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	msgs, err := s.store.GetLastMessages(s.ownerID(), limit)
	if err != nil {
		log.Printf("web: get messages: %v", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get messages")
		return
	}

	type msgDTO struct {
		ID        int64  `json:"id"`
		Role      string `json:"role"`
		Text      string `json:"text"`
		CreatedAt string `json:"created_at"`
	}

	result := make([]msgDTO, 0, len(msgs))
	for _, m := range msgs {
		result = append(result, msgDTO{
			ID:        m.ID,
			Role:      m.Role,
			Text:      m.Text,
			CreatedAt: m.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	s.writeJSON(w, result)
}

// handleConfig handles GET (current config) and POST (update scheduler settings).
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, map[string]any{
			"scheduler_enabled": s.cfg.Scheduler.Enabled,
			"scheduler_min_hour": s.cfg.Scheduler.MinHour,
			"scheduler_max_hour": s.cfg.Scheduler.MaxHour,
			"web_port":           s.cfg.Web.Port,
		})

	case http.MethodPost:
		// Config updates are informational only — actual changes require restart.
		// This endpoint acknowledges the request for UI feedback.
		s.writeJSON(w, map[string]string{
			"status":  "ok",
			"message": "config changes require bot restart",
		})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
