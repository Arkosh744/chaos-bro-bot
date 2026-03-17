package bot

import (
	"fmt"
	"log"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/groq"
	"github.com/Arkosh744/chaos-bro-bot/internal/scheduler"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/internal/web"
	tele "gopkg.in/telebot.v4"
)

type Bot struct {
	tg                    *tele.Bot
	claude                *claude.Client
	whisper               *groq.WhisperClient
	store                 *storage.Storage
	scheduler             *scheduler.Scheduler
	web                   *web.Server
	ownerID               int64
	groupInterjectChance  int
}

var menu = &tele.ReplyMarkup{ResizeKeyboard: true}
var menuMore = &tele.ReplyMarkup{ResizeKeyboard: true}

var (
	// Main menu buttons
	btnGround    = menu.Text("👁 Очнись")
	btnChaos     = menu.Text("🎲 Ебани куба")
	btnPredict   = menu.Text("🔮 Судьба")
	btnRandomize = menu.Text("🎱 Кинь кости")
	btnBreathe   = menu.Text("🫁 Дыши")
	btnMore      = menu.Text("➡️ Ещё")

	// Secondary menu buttons
	btnRoast     = menuMore.Text("🔥 Зажарь")
	btnWisdom    = menuMore.Text("🧙 Мудрость")
	btnHoroscope = menuMore.Text("⭐ Гороскоп")
	btnMood      = menuMore.Text("📊 Настроение")
	btnMirror    = menuMore.Text("🪞 Зеркало")
	btnBack      = menuMore.Text("⬅️ Назад")
)

var inlineMenu = &tele.ReplyMarkup{}
var (
	btnMoreGround = inlineMenu.Data("🔄 Ещё", "more_ground")
	btnMoreChaos  = inlineMenu.Data("🔄 Другое", "more_chaos")
)

func New(token string, ownerID int64, cl *claude.Client, whisper *groq.WhisperClient, store *storage.Storage, schedCfg scheduler.Config, cfg interface{}, webSrv *web.Server, groupInterjectChance int) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 30 * time.Second},
	}

	tg, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	menu.Reply(
		menu.Row(btnGround, btnChaos, btnPredict),
		menu.Row(btnRandomize, btnBreathe, btnMore),
	)
	menuMore.Reply(
		menuMore.Row(btnRoast, btnWisdom, btnHoroscope),
		menuMore.Row(btnMood, btnMirror, btnBack),
	)

	b := &Bot{
		tg:                   tg,
		claude:               cl,
		whisper:              whisper,
		store:                store,
		ownerID:              ownerID,
		web:                  webSrv,
		groupInterjectChance: groupInterjectChance,
	}

	b.scheduler = scheduler.New(schedCfg, tg, cl, store)

	// Wire scheduler and send function into web server if it was started early
	if b.web != nil {
		b.web.SetScheduler(b.scheduler)
		b.web.SetSendFunc(func(userID int64, text string) error {
			_, err := b.tg.Send(&tele.User{ID: userID}, text)
			return err
		})
	}

	b.registerHandlers()

	return b, nil
}

func (b *Bot) registerHandlers() {
	b.tg.Handle("/start", b.handleStart)
	b.tg.Handle("/help", b.handleHelp)
	b.tg.Handle(&btnGround, b.handleGrounding)
	b.tg.Handle(&btnChaos, b.handleChaos)
	b.tg.Handle(&btnRandomize, b.handleRandomize)
	b.tg.Handle(&btnPredict, b.handlePrediction)
	b.tg.Handle(&btnMoreGround, b.handleGroundingMore)
	b.tg.Handle(&btnMoreChaos, b.handleChaosMore)
	b.tg.Handle(&btnBreathe, b.handleBreathing)
	b.tg.Handle(&btnMore, func(c tele.Context) error {
		return c.Send("🎭 Дополнительные возможности:", menuMore)
	})
	b.tg.Handle(&btnBack, func(c tele.Context) error {
		return c.Send("👌", menu)
	})
	b.tg.Handle("/capsule", b.handleCapsule)
	for i := 1; i <= 10; i++ {
		score := i
		b.tg.Handle(&tele.InlineButton{Unique: fmt.Sprintf("mood_%d", i)}, func(c tele.Context) error {
			return b.handleMoodScore(c, score)
		})
	}
	b.tg.Handle(&btnRoast, b.handleRoast)
	b.tg.Handle(&btnWisdom, b.handleWisdom)
	b.tg.Handle(&btnHoroscope, b.handleHoroscope)
	b.tg.Handle(&btnMood, b.handleMoodGraph)
	b.tg.Handle(&btnMirror, b.handleMirror)
	b.tg.Handle("/mood", b.handleMoodGraph)
	b.tg.Handle("/achievements", b.handleAchievements)
	b.tg.Handle("/profile", b.handleProfile)
	b.tg.Handle("/truth", b.handleTruth)
	b.tg.Handle("/silence", b.handleSilence)
	b.tg.Handle("/mirror", b.handleMirror)
	b.tg.Handle("/roast", b.handleRoast)
	b.tg.Handle("/wisdom", b.handleWisdom)
	b.tg.Handle("/horoscope", b.handleHoroscope)
	b.tg.Handle("/level", b.handleLevel)
	b.tg.Handle("/remind", b.handleRemind)
	b.tg.Handle("/streak", b.handleStreak)
	b.tg.Handle("/trickster", b.handleTricksterIntro)
	b.tg.Handle(tele.OnPhoto, b.handlePhoto)
	b.tg.Handle(tele.OnText, b.handleText)
	b.tg.Handle(tele.OnVoice, b.handleVoice)
}

func (b *Bot) Start() {
	log.Println("Trickster bot started")

	if b.ownerID != 0 {
		owner := &tele.User{ID: b.ownerID}
		if _, err := b.tg.Send(owner, "Йо, я проснулся. Готов к хаосу."); err != nil {
			log.Printf("startup message: %v", err)
		}
	}

	b.scheduler.Start()
	b.tg.Start()
}

func (b *Bot) Stop() {
	b.scheduler.Stop()
	b.tg.Stop()
}
