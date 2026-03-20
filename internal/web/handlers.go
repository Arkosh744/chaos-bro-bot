package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
)

func (s *Server) ownerID() int64 {
	return s.cfg.Telegram.OwnerID
}

// getUserID reads user_id from query param or falls back to ownerID.
func (s *Server) getUserID(r *http.Request) int64 {
	if raw := r.URL.Query().Get("user_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id != 0 {
			return id
		}
	}
	return s.ownerID()
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

// handleUsers returns a list of all users with message count and last activity.
func (s *Server) handleUsers(w http.ResponseWriter, _ *http.Request) {
	users, err := s.store.GetAllUsers()
	if err != nil {
		log.Printf("web: get all users: %v", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get users")
		return
	}

	type userDTO struct {
		UserID       int64  `json:"user_id"`
		Username     string `json:"username"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		DisplayName  string `json:"display_name"`
		MessageCount int    `json:"message_count"`
		LastMessage  string `json:"last_message"`
		IsOwner      bool   `json:"is_owner"`
	}

	result := make([]userDTO, 0, len(users))
	for _, u := range users {
		var lastMsg string
		if !u.LastMessage.IsZero() {
			lastMsg = u.LastMessage.Format("2006-01-02 15:04:05")
		}
		// Build display name: prefer first_name, fallback to username, then ID
		displayName := u.FirstName
		if displayName == "" {
			displayName = u.Username
		}
		if displayName == "" {
			displayName = fmt.Sprintf("User %d", u.UserID)
		}
		if u.LastName != "" {
			displayName = displayName + " " + u.LastName
		}
		result = append(result, userDTO{
			UserID:       u.UserID,
			Username:     u.Username,
			FirstName:    u.FirstName,
			LastName:     u.LastName,
			DisplayName:  displayName,
			MessageCount: u.MessageCount,
			LastMessage:  lastMsg,
			IsOwner:      u.UserID == s.ownerID(),
		})
	}

	s.writeJSON(w, result)
}

// handleStats returns message counts and last activity time.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	uid := s.getUserID(r)

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
	uid := s.getUserID(r)

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	entries, err := s.store.GetMoodHistory(uid, days)
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
	uid := s.getUserID(r)

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
	uid := s.getUserID(r)

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

	// Append custom achievements
	customAchs, err := s.store.GetActiveCustomAchievements()
	if err != nil {
		log.Printf("web: get custom achievements: %v", err)
	}
	for _, ca := range customAchs {
		key := fmt.Sprintf("custom_%d", ca.ID)
		result = append(result, achDTO{
			Key:      key,
			Name:     ca.Name,
			Emoji:    ca.Emoji,
			Desc:     ca.Description,
			Unlocked: unlockedSet[key],
		})
	}

	s.writeJSON(w, result)
}

// handleMessages returns the last N messages (default 50).
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	uid := s.getUserID(r)

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	msgs, err := s.store.GetLastMessages(uid, limit)
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

// handleConfig handles GET for current config (including live scheduler state).
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	schedCfg := s.scheduler.GetConfig()
	s.writeJSON(w, map[string]any{
		"scheduler_enabled":  schedCfg.Enabled,
		"scheduler_min_hour": schedCfg.MinHour,
		"scheduler_max_hour": schedCfg.MaxHour,
		"web_port":           s.cfg.Web.Port,
	})
}

// handleConfigScheduler toggles the scheduler enabled/disabled at runtime.
func (s *Server) handleConfigScheduler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	s.scheduler.SetEnabled(req.Enabled)
	s.writeJSON(w, map[string]any{
		"status":  "ok",
		"enabled": req.Enabled,
	})
}

// handleSummary returns the bot's context summary for the selected user.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	uid := s.getUserID(r)

	summary, lastMessageID, err := s.store.GetSummary(uid)
	if err != nil {
		log.Printf("web: get summary: %v", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get summary")
		return
	}

	s.writeJSON(w, map[string]any{
		"summary":         summary,
		"last_message_id": lastMessageID,
	})
}

// handleSend sends a Telegram message to the specified user.
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.sendFunc == nil {
		s.writeError(w, http.StatusServiceUnavailable, "send function not initialized")
		return
	}

	var req struct {
		UserID int64  `json:"user_id"`
		Text   string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.UserID == 0 {
		req.UserID = s.ownerID()
	}
	if req.Text == "" {
		s.writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	if err := s.sendFunc(req.UserID, req.Text); err != nil {
		log.Printf("web: send message to %d: %v", req.UserID, err)
		s.writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	// Save bot message to storage
	if _, err := s.store.SaveMessage(req.UserID, "bot", req.Text); err != nil {
		log.Printf("web: save sent message: %v", err)
	}

	s.writeJSON(w, map[string]string{"status": "ok"})
}

// handleSchedulerPing triggers an immediate scheduler ping to the specified user.
func (s *Server) handleSchedulerPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.scheduler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler not initialized")
		return
	}

	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.UserID == 0 {
		req.UserID = s.ownerID()
	}

	go s.scheduler.SendPingNow(req.UserID)

	s.writeJSON(w, map[string]string{"status": "ok"})
}

// handleConfigHours updates the scheduler min/max hours at runtime.
func (s *Server) handleConfigHours(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		MinHour int `json:"min_hour"`
		MaxHour int `json:"max_hour"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if req.MinHour < 0 || req.MinHour > 23 || req.MaxHour < 0 || req.MaxHour > 23 {
		s.writeError(w, http.StatusBadRequest, "hours must be between 0 and 23")
		return
	}
	if req.MinHour >= req.MaxHour {
		s.writeError(w, http.StatusBadRequest, "min_hour must be less than max_hour")
		return
	}

	s.scheduler.SetHours(req.MinHour, req.MaxHour)
	s.writeJSON(w, map[string]any{
		"status":   "ok",
		"min_hour": req.MinHour,
		"max_hour": req.MaxHour,
	})
}

// handlePrompts handles GET (list all prompts) and POST (save/delete override).
func (s *Server) handlePrompts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		overrides, err := s.store.GetAllPromptOverrides()
		if err != nil {
			log.Printf("web: get prompt overrides: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to get prompt overrides")
			return
		}

		allPrompts := features.AllPrompts()

		type promptDTO struct {
			Name         string `json:"name"`
			DefaultValue string `json:"default_value"`
			Override     string `json:"override"`
		}

		result := make([]promptDTO, 0, len(allPrompts))
		for _, p := range allPrompts {
			result = append(result, promptDTO{
				Name:         p.Name,
				DefaultValue: p.DefaultValue,
				Override:     overrides[p.Name],
			})
		}
		s.writeJSON(w, result)

	case http.MethodPost:
		var req struct {
			Name   string `json:"name"`
			Value  string `json:"value"`
			Delete bool   `json:"delete"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Name == "" {
			s.writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		if req.Delete {
			log.Printf("web: prompt override deleted: %s", req.Name)
			if err := s.store.DeletePromptOverride(req.Name); err != nil {
				log.Printf("web: delete prompt override: %v", err)
				s.writeError(w, http.StatusInternalServerError, "failed to delete prompt override")
				return
			}
		} else {
			if req.Value == "" {
				s.writeError(w, http.StatusBadRequest, "value is required")
				return
			}
			log.Printf("web: prompt override: %s (len=%d)", req.Name, len(req.Value))
			if err := s.store.SavePromptOverride(req.Name, req.Value); err != nil {
				log.Printf("web: save prompt override: %v", err)
				s.writeError(w, http.StatusInternalServerError, "failed to save prompt override")
				return
			}
		}

		s.writeJSON(w, map[string]string{"status": "ok"})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleEasterEggs handles GET (list), POST (add), and DELETE (remove) for custom easter eggs.
func (s *Server) handleEasterEggs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		eggs, err := s.store.ListCustomEasterEggs()
		if err != nil {
			log.Printf("web: list easter eggs: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to list easter eggs")
			return
		}

		type eggDTO struct {
			ID       int64  `json:"id"`
			Trigger  string `json:"trigger"`
			Response string `json:"response"`
		}

		result := make([]eggDTO, 0, len(eggs))
		for _, e := range eggs {
			result = append(result, eggDTO{
				ID:       e.ID,
				Trigger:  e.Trigger,
				Response: e.Response,
			})
		}
		s.writeJSON(w, result)

	case http.MethodPost:
		var req struct {
			Trigger  string `json:"trigger"`
			Response string `json:"response"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Trigger == "" || req.Response == "" {
			s.writeError(w, http.StatusBadRequest, "trigger and response are required")
			return
		}

		if err := s.store.AddCustomEasterEgg(req.Trigger, req.Response); err != nil {
			log.Printf("web: add easter egg: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to add easter egg")
			return
		}
		s.writeJSON(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			s.writeError(w, http.StatusBadRequest, "id is required")
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		if err := s.store.DeleteCustomEasterEgg(id); err != nil {
			log.Printf("web: delete easter egg: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to delete easter egg")
			return
		}
		s.writeJSON(w, map[string]string{"status": "ok"})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleBackup creates a database backup and returns it as a downloadable file.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Create backup directory next to the DB file
	dbDir := filepath.Dir(s.store.DBPath())
	backupDir := filepath.Join(dbDir, "data", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		log.Printf("web: create backup dir: %v", err)
		s.writeError(w, http.StatusInternalServerError, "failed to create backup directory")
		return
	}

	filename := fmt.Sprintf("backup_%s.db", time.Now().Format("20060102_150405"))
	destPath := filepath.Join(backupDir, filename)

	if err := s.store.Backup(destPath); err != nil {
		log.Printf("web: backup: %v", err)
		s.writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	log.Printf("web: backup created: %s", destPath)
	log.Printf("web: backup downloaded by auth user")

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeFile(w, r, destPath)

	// Clean up temp backup file after serving
	go func() {
		// Small delay to ensure file is fully sent
		time.Sleep(5 * time.Second)
		if err := os.Remove(destPath); err != nil {
			log.Printf("web: remove backup temp file: %v", err)
		}
	}()
}

// handleAnalytics returns comprehensive analytics for a user.
func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	uid := s.getUserID(r)

	// Messages by day (last 30 days)
	messagesByDay, err := s.store.GetMessagesByDay(uid, 30)
	if err != nil {
		log.Printf("web: get messages by day: %v", err)
	}
	type dayCountDTO struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	daysResult := make([]dayCountDTO, 0, len(messagesByDay))
	for _, d := range messagesByDay {
		daysResult = append(daysResult, dayCountDTO{Date: d.Date, Count: d.Count})
	}

	// Top commands
	topCommands, err := s.store.GetTopCommands(uid, 10)
	if err != nil {
		log.Printf("web: get top commands: %v", err)
	}
	type cmdCountDTO struct {
		Command string `json:"command"`
		Count   int    `json:"count"`
	}
	cmdsResult := make([]cmdCountDTO, 0, len(topCommands))
	for _, c := range topCommands {
		cmdsResult = append(cmdsResult, cmdCountDTO{Command: c.Command, Count: c.Count})
	}

	// Activity streak (from counters)
	streak, _ := s.store.GetCounter(uid, "streak_days")

	// Average mood over 30 days
	moodEntries, err := s.store.GetMoodHistory(uid, 30)
	if err != nil {
		log.Printf("web: get mood history for analytics: %v", err)
	}
	var avgMood float64
	if len(moodEntries) > 0 {
		total := 0
		for _, e := range moodEntries {
			total += e.Score
		}
		avgMood = float64(total) / float64(len(moodEntries))
	}

	// Most active hour
	hourly, err := s.store.GetHourlyActivity(uid)
	if err != nil {
		log.Printf("web: get hourly for analytics: %v", err)
	}
	mostActiveHour := 0
	maxCount := 0
	for _, h := range hourly {
		if h.Count > maxCount {
			maxCount = h.Count
			mostActiveHour = h.Hour
		}
	}

	// Average word count
	avgWords, err := s.store.GetAverageWordCount(uid)
	if err != nil {
		log.Printf("web: get avg word count: %v", err)
	}

	// Response time average: approximate by looking at pairs of user/bot messages
	var responseTimeAvg float64
	msgs, err := s.store.GetLastMessages(uid, 200)
	if err != nil {
		log.Printf("web: get messages for response time: %v", err)
	} else {
		var totalDuration float64
		var count int
		for i := 1; i < len(msgs); i++ {
			if msgs[i-1].Role == "user" && msgs[i].Role == "bot" {
				diff := msgs[i].CreatedAt.Sub(msgs[i-1].CreatedAt).Seconds()
				if diff > 0 && diff < 300 { // Only count if < 5 min
					totalDuration += diff
					count++
				}
			}
		}
		if count > 0 {
			responseTimeAvg = totalDuration / float64(count)
		}
	}

	s.writeJSON(w, map[string]any{
		"messages_by_day":   daysResult,
		"top_commands":      cmdsResult,
		"activity_streak":   streak,
		"avg_mood":          avgMood,
		"response_time_avg": responseTimeAvg,
		"most_active_hour":  mostActiveHour,
		"word_count_avg":    avgWords,
	})
}

// handleCustomAchievements handles GET (list), POST (add), and DELETE (remove) for custom achievements.
func (s *Server) handleCustomAchievements(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		achievements, err := s.store.ListCustomAchievements()
		if err != nil {
			log.Printf("web: list custom achievements: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to list custom achievements")
			return
		}

		type achDTO struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Emoji       string `json:"emoji"`
			Description string `json:"description"`
			Event       string `json:"event"`
			Threshold   int    `json:"threshold"`
			Active      bool   `json:"active"`
		}

		result := make([]achDTO, 0, len(achievements))
		for _, a := range achievements {
			result = append(result, achDTO{
				ID:          a.ID,
				Name:        a.Name,
				Emoji:       a.Emoji,
				Description: a.Description,
				Event:       a.Event,
				Threshold:   a.Threshold,
				Active:      a.Active,
			})
		}
		s.writeJSON(w, result)

	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Emoji       string `json:"emoji"`
			Description string `json:"description"`
			Event       string `json:"event"`
			Threshold   int    `json:"threshold"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Name == "" || req.Description == "" || req.Event == "" {
			s.writeError(w, http.StatusBadRequest, "name, description, and event are required")
			return
		}
		if req.Emoji == "" {
			req.Emoji = "\U0001F3C6"
		}
		if req.Threshold < 1 {
			req.Threshold = 1
		}

		if err := s.store.AddCustomAchievement(req.Name, req.Emoji, req.Description, req.Event, req.Threshold); err != nil {
			log.Printf("web: add custom achievement: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to add custom achievement")
			return
		}
		s.writeJSON(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			s.writeError(w, http.StatusBadRequest, "id is required")
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		if err := s.store.DeleteCustomAchievement(id); err != nil {
			log.Printf("web: delete custom achievement: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to delete custom achievement")
			return
		}
		s.writeJSON(w, map[string]string{"status": "ok"})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// RuntimeConfigDef describes a single overridable runtime config key with its metadata.
type RuntimeConfigDef struct {
	Key          string `json:"key"`
	Description  string `json:"description"`
	DefaultValue string `json:"default_value"`
}

// runtimeConfigDefs lists all known overridable runtime config keys.
var runtimeConfigDefs = []RuntimeConfigDef{
	{Key: "interject_chance", Description: "Group interject chance (0-100%)", DefaultValue: "10"},
	{Key: "rate_limit_per_hour", Description: "Max Claude calls per hour per user", DefaultValue: "30"},
	{Key: "bargain_chance", Description: "Bargain message chance (0-100%)", DefaultValue: "20"},
}

// handleRuntimeConfig handles GET (all runtime config) and POST (set key/value).
func (s *Server) handleRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		stored, err := s.store.GetAllRuntimeConfig()
		if err != nil {
			log.Printf("web: get runtime config: %v", err)
			s.writeError(w, http.StatusInternalServerError, "failed to get runtime config")
			return
		}

		type configDTO struct {
			Key          string `json:"key"`
			Value        string `json:"value"`
			DefaultValue string `json:"default_value"`
			Description  string `json:"description"`
			IsOverridden bool   `json:"is_overridden"`
		}

		result := make([]configDTO, 0, len(runtimeConfigDefs))
		for _, def := range runtimeConfigDefs {
			val, overridden := stored[def.Key]
			if !overridden {
				val = def.DefaultValue
			}
			result = append(result, configDTO{
				Key:          def.Key,
				Value:        val,
				DefaultValue: def.DefaultValue,
				Description:  def.Description,
				IsOverridden: overridden,
			})
		}
		s.writeJSON(w, result)

	case http.MethodPost:
		var req struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Delete bool   `json:"delete"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Key == "" {
			s.writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		// Validate key is known
		known := false
		for _, def := range runtimeConfigDefs {
			if def.Key == req.Key {
				known = true
				break
			}
		}
		if !known {
			s.writeError(w, http.StatusBadRequest, "unknown config key: "+req.Key)
			return
		}

		if req.Delete {
			if err := s.store.DeleteRuntimeConfig(req.Key); err != nil {
				log.Printf("web: delete runtime config: %v", err)
				s.writeError(w, http.StatusInternalServerError, "failed to delete runtime config")
				return
			}
		} else {
			if req.Value == "" {
				s.writeError(w, http.StatusBadRequest, "value is required")
				return
			}
			if err := s.store.SetRuntimeConfig(req.Key, req.Value); err != nil {
				log.Printf("web: set runtime config: %v", err)
				s.writeError(w, http.StatusInternalServerError, "failed to set runtime config")
				return
			}
		}

		s.writeJSON(w, map[string]string{"status": "ok"})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
