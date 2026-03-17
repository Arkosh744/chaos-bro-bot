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
	tg        *tele.Bot
	claude    *claude.Client
	whisper   *groq.WhisperClient
	store     *storage.Storage
	scheduler *scheduler.Scheduler
	web       *web.Server
	ownerID   int64
}

var menu = &tele.ReplyMarkup{ResizeKeyboard: true}

var (
	btnGround    = menu.Text("👁 Очнись")
	btnChaos     = menu.Text("🎲 Ебани куба")
	btnRandomize = menu.Text("🎱 Кинь кости")
	btnPredict   = menu.Text("🔮 Судьба")
	btnBreathe   = menu.Text("🫁 Дыши")
	btnRoast     = menu.Text("🔥 Зажарь")
	btnWisdom    = menu.Text("🧙 Мудрость")
	btnHoroscope = menu.Text("⭐ Гороскоп")
	btnMood      = menu.Text("📊 Настроение")
	btnMirror    = menu.Text("🪞 Зеркало")
)

var inlineMenu = &tele.ReplyMarkup{}
var (
	btnMoreGround = inlineMenu.Data("🔄 Ещё", "more_ground")
	btnMoreChaos  = inlineMenu.Data("🔄 Другое", "more_chaos")
)

func New(token string, ownerID int64, cl *claude.Client, whisper *groq.WhisperClient, store *storage.Storage, schedCfg scheduler.Config, cfg interface{}, webSrv *web.Server) (*Bot, error) {
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
		menu.Row(btnRandomize, btnRoast, btnWisdom),
		menu.Row(btnHoroscope, btnMood, btnMirror),
		menu.Row(btnBreathe),
	)

	b := &Bot{
		tg:      tg,
		claude:  cl,
		whisper: whisper,
		store:   store,
		ownerID: ownerID,
		web:     webSrv,
	}

	b.scheduler = scheduler.New(schedCfg, tg, cl, store)

	// Wire scheduler into web server if it was started early
	if b.web != nil {
		b.web.SetScheduler(b.scheduler)
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
