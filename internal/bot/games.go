package bot

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	tele "gopkg.in/telebot.v4"
)

// Game type constants stored in counter "game_active".
const (
	gameNone    = 0
	gameGuess   = 1
	gameDanetki = 3
	gameTrivia  = 4
)

// handleGame shows the game selection menu or processes subcommands.
func (b *Bot) handleGame(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /game: %s", userID, payload)

	if payload == "stop" {
		return b.handleGameStop(c)
	}
	if payload == "score" {
		return b.handleGameScore(c)
	}

	inline := &tele.ReplyMarkup{}
	inline.Inline(
		inline.Row(
			inline.Data("🔢 Угадай число", "game_guess"),
			inline.Data("🤔 Правда/Действие", "game_td"),
		),
		inline.Row(
			inline.Data("🧩 Данетки", "game_danetki"),
			inline.Data("📚 Тривиа", "game_trivia"),
		),
	)

	return c.Send("🎮 Выбирай игру:", inline)
}

// handleGameStop ends the current active game.
func (b *Bot) handleGameStop(c tele.Context) error {
	userID := c.Sender().ID
	gameType, _ := b.store.GetCounter(userID, "game_active")

	if gameType == gameNone {
		return c.Send("Нет активной игры.", menu)
	}

	// If danetki — reveal the answer
	if gameType == gameDanetki {
		answer, _ := b.store.GetFact(userID, "game_danetki_answer")
		b.clearGameState(userID)
		if answer != "" {
			return c.Send("🧩 Сдаёшься? Ладно.\n\nОтвет: "+answer, menu)
		}
		return c.Send("Игра завершена.", menu)
	}

	b.clearGameState(userID)
	return c.Send("Игра завершена.", menu)
}

// handleGameScore shows the trivia high score.
func (b *Bot) handleGameScore(c tele.Context) error {
	userID := c.Sender().ID
	score, _ := b.store.GetCounter(userID, "trivia_highscore")
	return c.Send(fmt.Sprintf("📚 Твой рекорд в тривии: %d", score), menu)
}

// clearGameState resets all game-related counters and facts.
func (b *Bot) clearGameState(userID int64) {
	b.store.SetCounter(userID, "game_active", gameNone)
	b.store.SetCounter(userID, "guess_target", 0)
	b.store.SetCounter(userID, "guess_attempts", 0)
	b.store.DeleteFact(userID, "game_danetki_riddle")
	b.store.DeleteFact(userID, "game_danetki_answer")
	b.store.DeleteFact(userID, "trivia_correct")
	b.store.SetCounter(userID, "trivia_score", 0)
}

// --- Game Start Handlers (inline button callbacks) ---

func (b *Bot) handleStartGuess(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] game: start guess", userID)

	target := rand.Intn(100) + 1
	b.store.SetCounter(userID, "game_active", gameGuess)
	b.store.SetCounter(userID, "guess_target", target)
	b.store.SetCounter(userID, "guess_attempts", 0)

	return c.Edit("🔢 Я загадал число от 1 до 100. У тебя 7 попыток. Пиши число!")
}

func (b *Bot) handleStartTruthDare(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] game: start truth_dare", userID)

	inline := &tele.ReplyMarkup{}
	inline.Inline(
		inline.Row(
			inline.Data("🤫 Правда", "td_truth"),
			inline.Data("🎬 Действие", "td_dare"),
		),
	)

	return c.Edit("🤔 Правда или действие?", inline)
}

func (b *Bot) handleTruthChoice(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] game: truth", userID)

	replyFn, stop := b.startThinking(c)
	result, err := b.claude.Ask(context.Background(), features.TruthPrompt, "Давай вопрос")
	if err != nil {
		stop()
		log.Printf("[%d] truth prompt error: %v", userID, err)
		return c.Send("Не получилось придумать вопрос. "+features.RandomFallback(), menu)
	}

	return replyFn("🤫 Правда:\n\n"+result, menu)
}

func (b *Bot) handleDareChoice(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] game: dare", userID)

	replyFn, stop := b.startThinking(c)
	result, err := b.claude.Ask(context.Background(), features.DarePrompt, "Давай действие")
	if err != nil {
		stop()
		log.Printf("[%d] dare prompt error: %v", userID, err)
		return c.Send("Не получилось придумать действие. "+features.RandomFallback(), menu)
	}

	return replyFn("🎬 Действие:\n\n"+result, menu)
}

func (b *Bot) handleStartDanetki(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] game: start danetki", userID)

	replyFn, stop := b.startThinking(c)
	result, err := b.claude.Ask(context.Background(), features.DanetkiPrompt, "Придумай данетку")
	if err != nil {
		stop()
		log.Printf("[%d] danetki generate error: %v", userID, err)
		return c.Send("Не получилось придумать загадку. "+features.RandomFallback(), menu)
	}

	riddle, answer := parseDanetki(result)
	if riddle == "" || answer == "" {
		stop()
		log.Printf("[%d] danetki parse failed: %s", userID, result)
		return c.Send("Не получилось придумать загадку. Попробуй ещё раз.", menu)
	}

	b.store.SetCounter(userID, "game_active", gameDanetki)
	b.store.SaveFact(userID, "game_danetki_riddle", riddle)
	b.store.SaveFact(userID, "game_danetki_answer", answer)

	return replyFn("🧩 Данетка:\n\n"+riddle+"\n\nЗадавай вопросы, на которые можно ответить Да/Нет.\n/game stop — сдаться и узнать ответ.", menu)
}

func (b *Bot) handleStartTrivia(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] game: start trivia", userID)

	b.store.SetCounter(userID, "game_active", gameTrivia)
	b.store.SetCounter(userID, "trivia_score", 0)

	return b.sendTriviaQuestion(c, userID)
}

func (b *Bot) sendTriviaQuestion(c tele.Context, userID int64) error {
	replyFn, stop := b.startThinking(c)
	result, err := b.claude.Ask(context.Background(), features.TriviaPrompt, "Задай вопрос для викторины")
	if err != nil {
		stop()
		log.Printf("[%d] trivia generate error: %v", userID, err)
		b.clearGameState(userID)
		return c.Send("Не получилось придумать вопрос. "+features.RandomFallback(), menu)
	}

	question, options, correct := parseTrivia(result)
	if question == "" || correct == "" {
		stop()
		log.Printf("[%d] trivia parse failed: %s", userID, result)
		b.clearGameState(userID)
		return c.Send("Не получилось придумать вопрос. Попробуй ещё раз.", menu)
	}

	// Store the correct answer
	b.store.SaveFact(userID, "trivia_correct", correct)

	score, _ := b.store.GetCounter(userID, "trivia_score")

	inline := &tele.ReplyMarkup{}
	var btns []tele.Btn
	for _, letter := range []string{"A", "B", "C", "D"} {
		if opt, ok := options[letter]; ok {
			btns = append(btns, inline.Data(letter+": "+opt, "trivia_"+letter))
		}
	}
	if len(btns) == 4 {
		inline.Inline(
			inline.Row(btns[0], btns[1]),
			inline.Row(btns[2], btns[3]),
		)
	}

	text := fmt.Sprintf("📚 Счёт: %d\n\n%s", score, question)
	return replyFn(text, inline)
}

func (b *Bot) handleTriviaAnswer(c tele.Context, letter string) error {
	userID := c.Sender().ID
	log.Printf("[%d] trivia answer: %s", userID, letter)

	gameType, _ := b.store.GetCounter(userID, "game_active")
	if gameType != gameTrivia {
		return c.Respond(&tele.CallbackResponse{Text: "Нет активной игры тривии"})
	}

	correct, _ := b.store.GetFact(userID, "trivia_correct")
	correct = strings.TrimSpace(strings.ToUpper(correct))

	if letter == correct {
		score, _ := b.store.GetCounter(userID, "trivia_score")
		score++
		b.store.SetCounter(userID, "trivia_score", score)

		if err := c.Respond(&tele.CallbackResponse{Text: "Правильно! +1"}); err != nil {
			log.Printf("[%d] trivia callback error: %v", userID, err)
		}

		return b.sendTriviaQuestion(c, userID)
	}

	// Wrong answer — game over
	score, _ := b.store.GetCounter(userID, "trivia_score")

	// Update highscore
	highscore, _ := b.store.GetCounter(userID, "trivia_highscore")
	if score > highscore {
		b.store.SetCounter(userID, "trivia_highscore", score)
	}

	b.clearGameState(userID)

	msg := fmt.Sprintf("❌ Неправильно! Правильный ответ: %s\n\nТвой счёт: %d", correct, score)
	if score > highscore {
		msg += " (новый рекорд! 🎉)"
	}

	return c.Edit(msg)
}

// --- Game Input Handler (dispatches text to active game) ---

func (b *Bot) handleGameInput(c tele.Context, gameType int, text string) error {
	switch gameType {
	case gameGuess:
		return b.handleGuessInput(c, text)
	case gameDanetki:
		return b.handleDanetkiInput(c, text)
	default:
		// Unknown game type, reset
		b.clearGameState(c.Sender().ID)
		return nil
	}
}

func (b *Bot) handleGuessInput(c tele.Context, text string) error {
	userID := c.Sender().ID

	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return c.Send("Напиши число от 1 до 100.", menu)
	}

	if num < 1 || num > 100 {
		return c.Send("От 1 до 100, дружище.", menu)
	}

	target, _ := b.store.GetCounter(userID, "guess_target")
	attempts, _ := b.store.GetCounter(userID, "guess_attempts")
	attempts++
	b.store.SetCounter(userID, "guess_attempts", attempts)

	if num == target {
		b.clearGameState(userID)
		return c.Send(fmt.Sprintf("🎉 Угадал за %d попыток! Число было %d.", attempts, target), menu)
	}

	if attempts >= 7 {
		b.clearGameState(userID)
		return c.Send(fmt.Sprintf("💀 Попытки кончились! Число было %d.", target), menu)
	}

	remaining := 7 - attempts
	if num < target {
		return c.Send(fmt.Sprintf("⬆️ Больше! (осталось %d попыток)", remaining), menu)
	}

	return c.Send(fmt.Sprintf("⬇️ Меньше! (осталось %d попыток)", remaining), menu)
}

func (b *Bot) handleDanetkiInput(c tele.Context, text string) error {
	userID := c.Sender().ID

	riddle, _ := b.store.GetFact(userID, "game_danetki_riddle")
	answer, _ := b.store.GetFact(userID, "game_danetki_answer")

	if riddle == "" || answer == "" {
		b.clearGameState(userID)
		return c.Send("Что-то пошло не так. Начни новую игру: /game", menu)
	}

	prompt := fmt.Sprintf(features.DanetkiJudgePrompt, riddle, answer, text)

	replyFn, stop := b.startThinking(c)
	result, err := b.claude.Ask(context.Background(), prompt, text)
	if err != nil {
		stop()
		log.Printf("[%d] danetki judge error: %v", userID, err)
		return c.Send("Не смог оценить вопрос. Попробуй другой.", menu)
	}

	// Normalize the response to one word
	result = strings.TrimSpace(result)
	normalized := strings.ToLower(result)
	switch {
	case strings.Contains(normalized, "да"):
		result = "✅ Да"
	case strings.Contains(normalized, "нет") && !strings.Contains(normalized, "неважно"):
		result = "❌ Нет"
	default:
		result = "🤷 Неважно"
	}

	return replyFn(result, menu)
}

// --- Parsers ---

func parseDanetki(raw string) (riddle, answer string) {
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "СИТУАЦИЯ|") {
			riddle = strings.TrimPrefix(line, "СИТУАЦИЯ|")
		}
		if strings.HasPrefix(line, "ОТВЕТ|") {
			answer = strings.TrimPrefix(line, "ОТВЕТ|")
		}
	}
	return riddle, answer
}

func parseTrivia(raw string) (question string, options map[string]string, correct string) {
	options = make(map[string]string)
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ВОПРОС|") {
			question = strings.TrimPrefix(line, "ВОПРОС|")
		}
		for _, letter := range []string{"A", "B", "C", "D"} {
			if strings.HasPrefix(line, letter+"|") {
				options[letter] = strings.TrimPrefix(line, letter+"|")
			}
		}
		if strings.HasPrefix(line, "ОТВЕТ|") {
			correct = strings.TrimSpace(strings.TrimPrefix(line, "ОТВЕТ|"))
		}
	}
	return question, options, correct
}
