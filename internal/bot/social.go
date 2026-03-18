package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	tele "gopkg.in/telebot.v4"
)

// --- Duel ---

// handleDuel initiates a duel in a group chat via reply to another user's message.
func (b *Bot) handleDuel(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send("Дуэли работают только в группах. Найди себе соперника.")
	}

	chatID := c.Chat().ID
	challengerID := c.Sender().ID
	log.Printf("[%d] /duel in chat %d", challengerID, chatID)

	// Must be a reply to someone's message
	if c.Message().ReplyTo == nil || c.Message().ReplyTo.Sender == nil {
		return c.Send("Ответь на сообщение того, кого хочешь вызвать на дуэль.")
	}

	opponent := c.Message().ReplyTo.Sender
	opponentID := opponent.ID

	// Cannot duel yourself
	if opponentID == challengerID {
		return c.Send("Дуэль с собой? Ты либо гений, либо одинок.")
	}

	// Cannot duel the bot
	if opponentID == b.tg.Me.ID {
		return c.Send("Не-не-не, я судья, а не участник.")
	}

	// Check for existing active duel
	existing, err := b.store.GetActiveDuel(chatID)
	if err != nil {
		log.Printf("[%d] get active duel error: %v", chatID, err)
		return c.Send("Что-то пошло не так. Попробуй позже.")
	}
	if existing != nil {
		return c.Send("В этом чате уже идёт дуэль. Дождись окончания.")
	}

	// Generate a duel question via Claude
	replyFn, stop := b.startThinking(c)
	question, err := b.claude.Ask(context.Background(), features.DuelQuestionPrompt, "Придумай вопрос для дуэли")
	if err != nil {
		stop()
		log.Printf("[%d] duel question generate error: %v", chatID, err)
		return c.Send("Не получилось придумать вопрос. " + features.RandomFallback())
	}

	duelID, err := b.store.CreateDuel(chatID, challengerID, opponentID, question)
	if err != nil {
		stop()
		log.Printf("[%d] create duel error: %v", chatID, err)
		return c.Send("Не удалось создать дуэль.")
	}

	challengerName := c.Sender().FirstName
	opponentName := opponent.FirstName

	log.Printf("[%d] duel #%d created: %s vs %s", chatID, duelID, challengerName, opponentName)

	msg := fmt.Sprintf("Duel!\n\n%s vs %s\n\nВопрос: %s\n\nОба пишите ответ прямо в чат. У вас 60 секунд!",
		challengerName, opponentName, question)

	// Auto-cancel duel after 60 seconds if both haven't answered
	go func() {
		time.Sleep(60 * time.Second)
		duel, err := b.store.GetActiveDuel(chatID)
		if err != nil || duel == nil || duel.Status != "pending" {
			return
		}
		b.store.CompleteDuel(duel.ID, 0)
		b.tg.Send(&tele.Chat{ID: chatID}, "⏰ Время вышло! Дуэль отменена — оба слишком медленные.")
	}()

	return replyFn(msg)
}

// handleDuelAnswer checks if a group message is a duel answer.
// Returns true if the message was consumed as a duel answer.
func (b *Bot) handleDuelAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	userID := c.Sender().ID

	duel, err := b.store.GetActiveDuel(chatID)
	if err != nil || duel == nil {
		return false
	}

	// Check if sender is a participant
	isChallenger := userID == duel.ChallengerID
	isOpponent := userID == duel.OpponentID
	if !isChallenger && !isOpponent {
		return false
	}

	// Check if already answered
	if isChallenger && duel.ChallengerAnswer != "" {
		return false
	}
	if isOpponent && duel.OpponentAnswer != "" {
		return false
	}

	answer := c.Text()
	if err := b.store.SubmitDuelAnswer(duel.ID, userID, answer); err != nil {
		log.Printf("[%d] submit duel answer error: %v", chatID, err)
		return false
	}

	senderName := c.Sender().FirstName
	log.Printf("[%d] duel #%d: %s answered", chatID, duel.ID, senderName)

	// Refresh duel state after saving the answer
	if isChallenger {
		duel.ChallengerAnswer = answer
	} else {
		duel.OpponentAnswer = answer
	}

	// Check if both have answered
	if duel.ChallengerAnswer == "" || duel.OpponentAnswer == "" {
		if err := c.Send(fmt.Sprintf("%s ответил! Ждём второго участника...", senderName)); err != nil {
			log.Printf("[%d] duel waiting send error: %v", chatID, err)
		}
		return true
	}

	// Both answered: judge the duel
	go b.judgeDuel(c, duel)
	return true
}

// judgeDuel sends both answers to Claude for judging and announces the winner.
func (b *Bot) judgeDuel(c tele.Context, duel *storage.Duel) {
	chatID := duel.ChatID
	judgePrompt := fmt.Sprintf(features.DuelJudgePrompt, duel.Question, duel.ChallengerAnswer, duel.OpponentAnswer)

	result, err := b.claude.Ask(context.Background(), judgePrompt, "Суди дуэль")
	if err != nil {
		log.Printf("[%d] duel judge error: %v", chatID, err)
		if sendErr := c.Send("Не получилось рассудить дуэль. Ничья!"); sendErr != nil {
			log.Printf("[%d] duel judge send error: %v", chatID, sendErr)
		}
		b.store.CompleteDuel(duel.ID, 0)
		return
	}

	// Parse result: ПОБЕДИТЕЛЬ|N|explanation
	var winnerNum int
	var explanation string
	parts := strings.SplitN(result, "|", 3)
	if len(parts) >= 3 {
		numStr := strings.TrimSpace(parts[1])
		if numStr == "1" {
			winnerNum = 1
		} else if numStr == "2" {
			winnerNum = 2
		}
		explanation = strings.TrimSpace(parts[2])
	}

	var winnerID int64
	var winnerName string

	// Get names for the announcement
	_, challengerFirst, _, _ := b.store.GetUserProfile(duel.ChallengerID)
	_, opponentFirst, _, _ := b.store.GetUserProfile(duel.OpponentID)
	if challengerFirst == "" {
		challengerFirst = "Игрок 1"
	}
	if opponentFirst == "" {
		opponentFirst = "Игрок 2"
	}

	switch winnerNum {
	case 1:
		winnerID = duel.ChallengerID
		winnerName = challengerFirst
	case 2:
		winnerID = duel.OpponentID
		winnerName = opponentFirst
	default:
		winnerID = 0
		winnerName = "Никто (ничья)"
	}

	if err := b.store.CompleteDuel(duel.ID, winnerID); err != nil {
		log.Printf("[%d] complete duel error: %v", chatID, err)
	}

	msg := fmt.Sprintf("Результаты дуэли!\n\n"+
		"%s: %s\n%s: %s\n\n"+
		"Победитель: %s\n%s",
		challengerFirst, duel.ChallengerAnswer,
		opponentFirst, duel.OpponentAnswer,
		winnerName, explanation)

	if err := c.Send(msg); err != nil {
		log.Printf("[%d] duel result send error: %v", chatID, err)
	}

	log.Printf("[%d] duel #%d completed, winner: %s (%d)", chatID, duel.ID, winnerName, winnerID)
}

// --- Group Quest ---

// handleQuest generates and sends a quest to the group chat.
func (b *Bot) handleQuest(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send("Квесты работают только в группах.")
	}

	chatID := c.Chat().ID
	log.Printf("[%d] /quest in chat %d", c.Sender().ID, chatID)

	// Check for existing active quest
	existing, err := b.store.GetActiveQuest(chatID)
	if err != nil {
		log.Printf("[%d] get active quest error: %v", chatID, err)
		return c.Send("Что-то пошло не так.")
	}
	if existing != nil {
		return c.Send("Квест уже активен! Выполняй: " + existing.Quest)
	}

	replyFn, stop := b.startThinking(c)
	quest, err := b.claude.Ask(context.Background(), features.GroupQuestPrompt, "Придумай квест для группы")
	if err != nil {
		stop()
		log.Printf("[%d] quest generate error: %v", chatID, err)
		return c.Send("Не получилось придумать квест. " + features.RandomFallback())
	}

	_, err = b.store.CreateGroupQuest(chatID, quest, "")
	if err != nil {
		stop()
		log.Printf("[%d] create quest error: %v", chatID, err)
		return c.Send("Не удалось создать квест.")
	}

	log.Printf("[%d] quest created: %s", chatID, quest)
	return replyFn(fmt.Sprintf("Квест!\n\n%s\n\nПервый кто выполнит — побеждает. Пишите ответ прямо в чат!", quest))
}

// handleQuestAnswer checks if a group message completes an active quest.
// Returns true if the message was consumed as a quest attempt.
func (b *Bot) handleQuestAnswer(c tele.Context) bool {
	chatID := c.Chat().ID

	quest, err := b.store.GetActiveQuest(chatID)
	if err != nil || quest == nil {
		return false
	}

	text := c.Text()
	userID := c.Sender().ID

	// Ask Claude to judge if the answer completes the quest
	judgePrompt := fmt.Sprintf(features.GroupQuestJudgePrompt, quest.Quest, text)
	result, err := b.claude.Ask(context.Background(), judgePrompt, "Оцени ответ")
	if err != nil {
		log.Printf("[%d] quest judge error: %v", chatID, err)
		return false
	}

	// Parse: РЕЗУЛЬТАТ|да or РЕЗУЛЬТАТ|нет
	result = strings.TrimSpace(strings.ToLower(result))
	if !strings.Contains(result, "|да") && !strings.Contains(result, "| да") {
		return false
	}

	// Winner found
	if err := b.store.CompleteQuest(quest.ID, userID); err != nil {
		log.Printf("[%d] complete quest error: %v", chatID, err)
	}

	winnerName := c.Sender().FirstName
	log.Printf("[%d] quest completed by %s (%d)", chatID, winnerName, userID)

	msg := fmt.Sprintf("Квест выполнен!\n\n%s справился первым!\n\nКвест был: %s", winnerName, quest.Quest)
	if err := c.Send(msg); err != nil {
		log.Printf("[%d] quest complete send error: %v", chatID, err)
	}

	return true
}

// --- Linked Users ---

// handleLink creates or confirms a link between two users.
func (b *Bot) handleLink(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /link: %s", userID, payload)

	if payload == "" {
		// Check if replying to someone's message
		if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
			target := c.Message().ReplyTo.Sender
			return b.processLink(c, userID, target.ID, target.FirstName)
		}
		return c.Send("Формат: /link (ответом на сообщение) или /link @username")
	}

	// Parse @username from payload
	username := strings.TrimPrefix(strings.TrimSpace(payload), "@")
	if username == "" {
		return c.Send("Формат: /link @username")
	}

	// Look up user by username in profiles
	targetID, err := b.findUserByUsername(username)
	if err != nil || targetID == 0 {
		return c.Send("Не нашёл пользователя. Он должен хотя бы раз написать боту.")
	}

	if targetID == userID {
		return c.Send("Нельзя связаться с собой. Хотя, понимаю желание.")
	}

	_, firstName, _, _ := b.store.GetUserProfile(targetID)
	if firstName == "" {
		firstName = username
	}

	return b.processLink(c, userID, targetID, firstName)
}

// processLink handles the link logic: create request or confirm existing.
func (b *Bot) processLink(c tele.Context, userID, targetID int64, targetName string) error {
	// Check if already linked
	existingPartner, err := b.store.GetLinkedUser(userID)
	if err != nil {
		log.Printf("[%d] get linked user error: %v", userID, err)
		return c.Send("Что-то пошло не так.")
	}
	if existingPartner != 0 {
		return c.Send("Ты уже связан с кем-то. Сначала /unlink.")
	}

	// Check if target sent us a pending request
	hasPending, err := b.store.GetPendingLink(targetID, userID)
	if err != nil {
		log.Printf("[%d] get pending link error: %v", userID, err)
		return c.Send("Что-то пошло не так.")
	}

	if hasPending {
		// Confirm the link
		if err := b.store.AcceptLink(targetID, userID); err != nil {
			log.Printf("[%d] accept link error: %v", userID, err)
			return c.Send("Не удалось подтвердить связь.")
		}

		senderName := c.Sender().FirstName
		log.Printf("[%d] link confirmed with %d", userID, targetID)

		// Notify the other user
		notifyMsg := fmt.Sprintf("%s принял твой запрос на связь! Теперь вы связаны.", senderName)
		recipient := &tele.User{ID: targetID}
		if _, err := b.tg.Send(recipient, notifyMsg); err != nil {
			log.Printf("[%d] link notify error: %v", targetID, err)
		}

		return c.Send(fmt.Sprintf("Связь с %s установлена!", targetName))
	}

	// Check if we already sent a pending request
	alreadySent, err := b.store.GetPendingLink(userID, targetID)
	if err != nil {
		log.Printf("[%d] check existing link error: %v", userID, err)
		return c.Send("Что-то пошло не так.")
	}
	if alreadySent {
		return c.Send("Ты уже отправил запрос. Жди подтверждения.")
	}

	// Create new pending link
	if err := b.store.CreateLink(userID, targetID); err != nil {
		log.Printf("[%d] create link error: %v", userID, err)
		return c.Send("Не удалось создать запрос.")
	}

	senderName := c.Sender().FirstName
	log.Printf("[%d] link request sent to %d", userID, targetID)

	// Notify target
	notifyMsg := fmt.Sprintf("%s хочет связать ваши аккаунты! Напиши /link @%s чтобы подтвердить.",
		senderName, c.Sender().Username)
	recipient := &tele.User{ID: targetID}
	if _, err := b.tg.Send(recipient, notifyMsg); err != nil {
		log.Printf("[%d] link request notify error: %v", targetID, err)
	}

	return c.Send(fmt.Sprintf("Запрос на связь отправлен %s. Ждём подтверждения.", targetName))
}

// handleUnlink removes a link between the user and their partner.
func (b *Bot) handleUnlink(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /unlink", userID)

	partnerID, err := b.store.GetLinkedUser(userID)
	if err != nil {
		log.Printf("[%d] get linked user error: %v", userID, err)
		return c.Send("Что-то пошло не так.")
	}

	if partnerID == 0 {
		return c.Send("Ты ни с кем не связан.")
	}

	if err := b.store.DeleteLink(userID, partnerID); err != nil {
		log.Printf("[%d] delete link error: %v", userID, err)
		return c.Send("Не удалось разорвать связь.")
	}

	// Notify partner
	senderName := c.Sender().FirstName
	notifyMsg := fmt.Sprintf("%s разорвал связь.", senderName)
	recipient := &tele.User{ID: partnerID}
	if _, err := b.tg.Send(recipient, notifyMsg); err != nil {
		log.Printf("[%d] unlink notify error: %v", partnerID, err)
	}

	log.Printf("[%d] unlinked from %d", userID, partnerID)
	return c.Send("Связь разорвана.")
}

// findUserByUsername searches the user_profiles table by username.
func (b *Bot) findUserByUsername(username string) (int64, error) {
	var userID int64
	err := b.store.FindUserByUsername(username, &userID)
	if err != nil {
		return 0, err
	}
	return userID, nil
}
