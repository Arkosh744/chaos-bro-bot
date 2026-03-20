package bot

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
	tele "gopkg.in/telebot.v4"
)

const (
	gameNone    = 0
	gameGuess   = 1
	gameDanetki = 3
	gameTrivia  = 4
)

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
			inline.Data(models.BtnGameGuess, "game_guess"),
			inline.Data(models.BtnGameTD, "game_td"),
		),
		inline.Row(
			inline.Data(models.BtnGameDanetki, "game_danetki"),
			inline.Data(models.BtnGameTrivia, "game_trivia"),
		),
	)

	return c.Send(models.MsgGameChoose, inline)
}

func (b *Bot) handleGameStop(c tele.Context) error {
	userID := c.Sender().ID
	gameType, _ := b.store.GetCounter(userID, "game_active")

	if gameType == gameNone {
		return c.Send(models.MsgGameNoActive, menu)
	}

	if gameType == gameDanetki {
		answer, _ := b.store.GetFact(userID, "game_danetki_answer")
		b.clearGameState(userID)
		if answer != "" {
			return c.Send(fmt.Sprintf(models.FmtDanetkiGiveUp, answer), menu)
		}
		return c.Send(models.MsgGameOver, menu)
	}

	b.clearGameState(userID)
	return c.Send(models.MsgGameOver, menu)
}

func (b *Bot) handleGameScore(c tele.Context) error {
	userID := c.Sender().ID
	score, _ := b.store.GetCounter(userID, "trivia_highscore")
	return c.Send(fmt.Sprintf(models.FmtTriviaHighscore, score), menu)
}

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

	return c.Edit(models.MsgGuessStart)
}

func (b *Bot) handleStartTruthDare(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] game: start truth_dare", userID)

	inline := &tele.ReplyMarkup{}
	inline.Inline(
		inline.Row(
			inline.Data(models.BtnTDTruth, "td_truth"),
			inline.Data(models.BtnTDDare, "td_dare"),
		),
	)

	return c.Edit(models.MsgTDChoose, inline)
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

	return replyFn(models.MsgTruthPrefix+result, menu)
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

	return replyFn(models.MsgDarePrefix+result, menu)
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
		return c.Send(models.MsgDanetkiParseFail, menu)
	}

	b.store.SetCounter(userID, "game_active", gameDanetki)
	b.store.SaveFact(userID, "game_danetki_riddle", riddle)
	b.store.SaveFact(userID, "game_danetki_answer", answer)

	return replyFn(fmt.Sprintf(models.MsgDanetkiStart, riddle), menu)
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
		return c.Send(models.MsgTriviaParseFail, menu)
	}

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

	text := fmt.Sprintf(models.FmtTriviaScore, score, question)
	return replyFn(text, inline)
}

func (b *Bot) handleTriviaAnswer(c tele.Context, letter string) error {
	userID := c.Sender().ID
	log.Printf("[%d] trivia answer: %s", userID, letter)

	gameType, _ := b.store.GetCounter(userID, "game_active")
	if gameType != gameTrivia {
		return c.Respond(&tele.CallbackResponse{Text: models.MsgTriviaNoActive})
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

		b.checkAndSendAchievements(c, "game_win")

		return b.sendTriviaQuestion(c, userID)
	}

	score, _ := b.store.GetCounter(userID, "trivia_score")
	highscore, _ := b.store.GetCounter(userID, "trivia_highscore")
	if score > highscore {
		b.store.SetCounter(userID, "trivia_highscore", score)
	}

	b.clearGameState(userID)

	msg := fmt.Sprintf(models.FmtTriviaWrong, correct, score)
	if score > highscore {
		msg += models.MsgTriviaNewRecord
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
		return c.Send(models.MsgGuessNaN, menu)
	}

	if num < 1 || num > 100 {
		return c.Send(models.MsgGuessRange, menu)
	}

	target, _ := b.store.GetCounter(userID, "guess_target")
	attempts, _ := b.store.GetCounter(userID, "guess_attempts")
	attempts++
	b.store.SetCounter(userID, "guess_attempts", attempts)

	if num == target {
		b.clearGameState(userID)
		return c.Send(fmt.Sprintf(models.FmtGuessCorrect, attempts, target), menu)
	}

	if attempts >= 7 {
		b.clearGameState(userID)
		return c.Send(fmt.Sprintf(models.FmtGuessLost, target), menu)
	}

	remaining := 7 - attempts
	if num < target {
		return c.Send(fmt.Sprintf(models.FmtGuessHigher, remaining), menu)
	}

	return c.Send(fmt.Sprintf(models.FmtGuessLower, remaining), menu)
}

func (b *Bot) handleDanetkiInput(c tele.Context, text string) error {
	userID := c.Sender().ID

	riddle, _ := b.store.GetFact(userID, "game_danetki_riddle")
	answer, _ := b.store.GetFact(userID, "game_danetki_answer")

	if riddle == "" || answer == "" {
		b.clearGameState(userID)
		return c.Send(models.MsgDanetkiBroken, menu)
	}

	systemPrompt := fmt.Sprintf(features.DanetkiJudgePrompt, riddle, answer)
	userMessage := "Ответ игрока: " + text

	replyFn, stop := b.startThinking(c)
	result, err := b.claude.Ask(context.Background(), systemPrompt, userMessage)
	if err != nil {
		stop()
		log.Printf("[%d] danetki judge error: %v", userID, err)
		return c.Send(models.MsgDanetkiBadJudge, menu)
	}

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
