package bot

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
	tele "gopkg.in/telebot.v4"
)

// --- Duel ---

func (b *Bot) handleDuel(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send(models.MsgDuelOnlyGroups)
	}

	chatID := c.Chat().ID
	challengerID := c.Sender().ID
	log.Printf("[%d] /duel in chat %d", challengerID, chatID)

	if c.Message().ReplyTo == nil || c.Message().ReplyTo.Sender == nil {
		return c.Send(models.MsgDuelReplyNeeded)
	}

	opponent := c.Message().ReplyTo.Sender
	opponentID := opponent.ID

	if opponentID == challengerID {
		return c.Send(models.MsgDuelSelf)
	}

	if opponentID == b.tg.Me.ID {
		return c.Send(models.MsgDuelWithBot)
	}

	existing, err := b.store.GetActiveDuel(chatID)
	if err != nil {
		log.Printf("[%d] get active duel error: %v", chatID, err)
		return c.Send(models.MsgDuelError)
	}
	if existing != nil {
		return c.Send(models.MsgDuelActive)
	}

	// Store opponent ID for category callback
	b.store.SetCounter(challengerID, "duel_opponent", int(opponentID))

	// Show category selection
	inline := &tele.ReplyMarkup{}
	inline.Inline(
		inline.Row(
			inline.Data(models.BtnDuelKnowledge, "duel_cat_knowledge"),
			inline.Data(models.BtnDuelHumor, "duel_cat_humor"),
		),
		inline.Row(
			inline.Data(models.BtnDuelGames, "duel_cat_games"),
			inline.Data(models.BtnDuelAbsurd, "duel_cat_absurd"),
		),
	)

	challengerName := c.Sender().FirstName
	opponentName := opponent.FirstName

	return c.Send(fmt.Sprintf(models.FmtDuelChallenge, challengerName, opponentName), inline)
}

func (b *Bot) handleDuelCategory(c tele.Context, category string) error {
	chatID := c.Chat().ID
	challengerID := c.Sender().ID

	opponentIDInt, _ := b.store.GetCounter(challengerID, "duel_opponent")
	opponentID := int64(opponentIDInt)
	if opponentID == 0 {
		return c.Edit(models.MsgDuelExpired)
	}

	// Clear stored opponent
	b.store.SetCounter(challengerID, "duel_opponent", 0)

	existing, err := b.store.GetActiveDuel(chatID)
	if err != nil {
		log.Printf("[%d] duel category check error: %v", chatID, err)
		return c.Edit(models.MsgDuelError)
	}
	if existing != nil {
		return c.Edit(models.MsgDuelActive)
	}

	categoryPrompts := models.DuelCategoryPrompts

	prompt := features.DuelQuestionPrompt
	if catPrompt, ok := categoryPrompts[category]; ok {
		prompt = catPrompt + " Одно предложение. На русском."
	}

	replyFn, stop := b.startThinking(c)
	question, err := b.claude.Ask(context.Background(), prompt, "Придумай вопрос для дуэли")
	if err != nil {
		stop()
		log.Printf("[%d] duel question generate error: %v", chatID, err)
		return c.Send("Не получилось придумать вопрос. " + features.RandomFallback())
	}

	duelID, err := b.store.CreateDuel(chatID, challengerID, opponentID, question)
	if err != nil {
		stop()
		log.Printf("[%d] create duel error: %v", chatID, err)
		return c.Send(models.MsgDuelCreateFailed)
	}

	_, challengerFirst, _, _ := b.store.GetUserProfile(challengerID)
	_, opponentFirst, _, _ := b.store.GetUserProfile(opponentID)
	if challengerFirst == "" {
		challengerFirst = models.MsgDuelDefaultPlayer1
	}
	if opponentFirst == "" {
		opponentFirst = models.MsgDuelDefaultPlayer2
	}

	log.Printf("[%d] duel #%d created: %s vs %s (cat: %s)", chatID, duelID, challengerFirst, opponentFirst, category)

	msg := fmt.Sprintf(models.FmtDuelQuestion,
		challengerFirst, opponentFirst, question)

	go func() {
		time.Sleep(60 * time.Second)
		duel, err := b.store.GetActiveDuel(chatID)
		if err != nil || duel == nil || duel.Status != "pending" {
			return
		}
		b.store.CompleteDuel(duel.ID, 0)
		b.tg.Send(&tele.Chat{ID: chatID}, models.MsgDuelTimedOut)
	}()

	return replyFn(msg)
}

func (b *Bot) handleDuelAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	userID := c.Sender().ID

	duel, err := b.store.GetActiveDuel(chatID)
	if err != nil || duel == nil {
		return false
	}

	isChallenger := userID == duel.ChallengerID
	isOpponent := userID == duel.OpponentID
	if !isChallenger && !isOpponent {
		return false
	}

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

	if isChallenger {
		duel.ChallengerAnswer = answer
	} else {
		duel.OpponentAnswer = answer
	}

	if duel.ChallengerAnswer == "" || duel.OpponentAnswer == "" {
		if err := c.Send(fmt.Sprintf(models.FmtDuelWaiting, senderName)); err != nil {
			log.Printf("[%d] duel waiting send error: %v", chatID, err)
		}
		return true
	}

	go b.judgeDuel(c, duel)
	return true
}

func (b *Bot) judgeDuel(c tele.Context, duel *storage.Duel) {
	chatID := duel.ChatID
	systemPrompt := fmt.Sprintf(features.DuelJudgePrompt, duel.Question)
	userMessage := fmt.Sprintf("Ответ игрока 1: %s\nОтвет игрока 2: %s", duel.ChallengerAnswer, duel.OpponentAnswer)

	result, err := b.claude.Ask(context.Background(), systemPrompt, userMessage)
	if err != nil {
		log.Printf("[%d] duel judge error: %v", chatID, err)
		if sendErr := c.Send(models.MsgDuelJudgeFailed); sendErr != nil {
			log.Printf("[%d] duel judge send error: %v", chatID, sendErr)
		}
		b.store.CompleteDuel(duel.ID, 0)
		return
	}

	result = strings.TrimSpace(strings.ToUpper(result))
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

	_, challengerFirst, _, _ := b.store.GetUserProfile(duel.ChallengerID)
	_, opponentFirst, _, _ := b.store.GetUserProfile(duel.OpponentID)
	if challengerFirst == "" {
		challengerFirst = models.MsgDuelDefaultPlayer1
	}
	if opponentFirst == "" {
		opponentFirst = models.MsgDuelDefaultPlayer2
	}

	switch winnerNum {
	case 1:
		winnerID = duel.ChallengerID
		winnerName = challengerFirst
	case 2:
		winnerID = duel.OpponentID
		winnerName = opponentFirst
	default:
		if rand.Intn(2) == 0 {
			winnerID = duel.ChallengerID
			winnerName = challengerFirst
		} else {
			winnerID = duel.OpponentID
			winnerName = opponentFirst
		}
		log.Printf("duel judge parse failed, random winner: %d", winnerID)
		if explanation == "" {
			explanation = models.MsgDuelRandomJudge
		}
	}

	if err := b.store.CompleteDuel(duel.ID, winnerID); err != nil {
		log.Printf("[%d] complete duel error: %v", chatID, err)
	}

	if winnerID != 0 {
		unlocked := features.CheckAchievements(b.store, winnerID, "duel_win")
		for _, achMsg := range unlocked {
			if _, sendErr := b.tg.Send(&tele.Chat{ID: chatID}, achMsg); sendErr != nil {
				log.Printf("[%d] duel achievement send error: %v", chatID, sendErr)
			}
		}
	}

	msg := fmt.Sprintf(models.FmtDuelResult,
		challengerFirst, duel.ChallengerAnswer,
		opponentFirst, duel.OpponentAnswer,
		winnerName, explanation)

	if err := c.Send(msg); err != nil {
		log.Printf("[%d] duel result send error: %v", chatID, err)
	}

	log.Printf("[%d] duel #%d completed, winner: %s (%d)", chatID, duel.ID, winnerName, winnerID)
}

// --- Group Quest ---

func (b *Bot) handleQuest(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send(models.MsgQuestOnlyGroups)
	}

	chatID := c.Chat().ID
	log.Printf("[%d] /quest in chat %d", c.Sender().ID, chatID)

	existing, err := b.store.GetActiveQuest(chatID)
	if err != nil {
		log.Printf("[%d] get active quest error: %v", chatID, err)
		return c.Send(models.MsgQuestError)
	}
	if existing != nil {
		return c.Send(models.MsgQuestActive + existing.Quest)
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
		return c.Send(models.MsgQuestCreateFail)
	}

	log.Printf("[%d] quest created: %s", chatID, quest)
	return replyFn(fmt.Sprintf(models.FmtQuestStart, quest))
}

func (b *Bot) handleQuestAnswer(c tele.Context) bool {
	chatID := c.Chat().ID

	quest, err := b.store.GetActiveQuest(chatID)
	if err != nil || quest == nil {
		return false
	}

	text := c.Text()
	userID := c.Sender().ID

	// Throttle: only judge every 3rd message to save Claude calls
	countKey := fmt.Sprintf("quest_msgs_%d", quest.ID)
	count, _ := b.store.IncrementCounter(chatID, countKey)
	if count%3 != 0 {
		return false // skip judging this message
	}

	questSystemPrompt := fmt.Sprintf(features.GroupQuestJudgePrompt, quest.Quest)
	result, err := b.claude.Ask(context.Background(), questSystemPrompt, text)
	if err != nil {
		log.Printf("[%d] quest judge error: %v", chatID, err)
		return false
	}

	result = strings.TrimSpace(strings.ToLower(result))
	if !strings.Contains(result, "|да") && !strings.Contains(result, "| да") {
		return false
	}

	if err := b.store.CompleteQuest(quest.ID, userID); err != nil {
		log.Printf("[%d] complete quest error: %v", chatID, err)
	}

	winnerName := c.Sender().FirstName
	log.Printf("[%d] quest completed by %s (%d)", chatID, winnerName, userID)

	msg := fmt.Sprintf(models.FmtQuestComplete, winnerName, quest.Quest)
	if err := c.Send(msg); err != nil {
		log.Printf("[%d] quest complete send error: %v", chatID, err)
	}

	return true
}

// --- Linked Users ---

func (b *Bot) handleLink(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /link: %s", userID, payload)

	if payload == "" {
		if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
			target := c.Message().ReplyTo.Sender
			return b.processLink(c, userID, target.ID, target.FirstName)
		}
		return c.Send(models.MsgLinkFormatReply)
	}

	username := strings.TrimPrefix(strings.TrimSpace(payload), "@")
	if username == "" {
		return c.Send(models.MsgLinkFormatUsername)
	}

	targetID, err := b.findUserByUsername(username)
	if err != nil || targetID == 0 {
		return c.Send(models.MsgLinkNotFound)
	}

	if targetID == userID {
		return c.Send(models.MsgLinkSelf)
	}

	_, firstName, _, _ := b.store.GetUserProfile(targetID)
	if firstName == "" {
		firstName = username
	}

	return b.processLink(c, userID, targetID, firstName)
}

func (b *Bot) processLink(c tele.Context, userID, targetID int64, targetName string) error {
	existingPartner, err := b.store.GetLinkedUser(userID)
	if err != nil {
		log.Printf("[%d] get linked user error: %v", userID, err)
		return c.Send("Что-то пошло не так.")
	}
	if existingPartner != 0 {
		return c.Send(models.MsgLinkAlreadyLinked)
	}

	hasPending, err := b.store.GetPendingLink(targetID, userID)
	if err != nil {
		log.Printf("[%d] get pending link error: %v", userID, err)
		return c.Send("Что-то пошло не так.")
	}

	if hasPending {
		if err := b.store.AcceptLink(targetID, userID); err != nil {
			log.Printf("[%d] accept link error: %v", userID, err)
			return c.Send(models.MsgLinkConfirmFail)
		}

		senderName := c.Sender().FirstName
		log.Printf("[%d] link confirmed with %d", userID, targetID)

		notifyMsg := fmt.Sprintf(models.FmtLinkConfirmNotify, senderName)
		recipient := &tele.User{ID: targetID}
		if _, err := b.tg.Send(recipient, notifyMsg); err != nil {
			log.Printf("[%d] link notify error: %v", targetID, err)
		}

		return c.Send(fmt.Sprintf(models.FmtLinkConfirmed, targetName))
	}

	alreadySent, err := b.store.GetPendingLink(userID, targetID)
	if err != nil {
		log.Printf("[%d] check existing link error: %v", userID, err)
		return c.Send("Что-то пошло не так.")
	}
	if alreadySent {
		return c.Send(models.MsgLinkAlreadySent)
	}

	if err := b.store.CreateLink(userID, targetID); err != nil {
		log.Printf("[%d] create link error: %v", userID, err)
		return c.Send(models.MsgLinkCreateFail)
	}

	senderName := c.Sender().FirstName
	log.Printf("[%d] link request sent to %d", userID, targetID)

	notifyMsg := fmt.Sprintf(models.FmtLinkRequestNotify,
		senderName, c.Sender().Username)
	recipient := &tele.User{ID: targetID}
	if _, err := b.tg.Send(recipient, notifyMsg); err != nil {
		log.Printf("[%d] link request notify error: %v", targetID, err)
	}

	return c.Send(fmt.Sprintf(models.FmtLinkRequestSent, targetName))
}

func (b *Bot) handleUnlink(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /unlink", userID)

	partnerID, err := b.store.GetLinkedUser(userID)
	if err != nil {
		log.Printf("[%d] get linked user error: %v", userID, err)
		return c.Send("Что-то пошло не так.")
	}

	if partnerID == 0 {
		return c.Send(models.MsgUnlinkNotLinked)
	}

	if err := b.store.DeleteLink(userID, partnerID); err != nil {
		log.Printf("[%d] delete link error: %v", userID, err)
		return c.Send(models.MsgUnlinkFail)
	}

	senderName := c.Sender().FirstName
	notifyMsg := fmt.Sprintf(models.FmtUnlinkNotify, senderName)
	recipient := &tele.User{ID: partnerID}
	if _, err := b.tg.Send(recipient, notifyMsg); err != nil {
		log.Printf("[%d] unlink notify error: %v", partnerID, err)
	}

	log.Printf("[%d] unlinked from %d", userID, partnerID)
	return c.Send(models.MsgUnlinkDone)
}

func (b *Bot) findUserByUsername(username string) (int64, error) {
	var userID int64
	err := b.store.FindUserByUsername(username, &userID)
	if err != nil {
		return 0, err
	}
	return userID, nil
}
