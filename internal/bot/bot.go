package bot

import (
	"log"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/scheduler"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	tele "gopkg.in/telebot.v4"
)

type Bot struct {
	tg        *tele.Bot
	claude    *claude.Client
	store     *storage.Storage
	scheduler *scheduler.Scheduler
	ownerID   int64
}

var menu = &tele.ReplyMarkup{ResizeKeyboard: true}

var (
	btnGround    = menu.Text("👁 Очнись")
	btnChaos     = menu.Text("🎲 Ебани куба")
	btnRandomize = menu.Text("🎱 Кинь кости")
	btnPredict   = menu.Text("🔮 Судьба")
)

var inlineMenu = &tele.ReplyMarkup{}
var (
	btnMoreGround = inlineMenu.Data("🔄 Ещё", "more_ground")
	btnMoreChaos  = inlineMenu.Data("🔄 Другое", "more_chaos")
)

func New(token string, ownerID int64, cl *claude.Client, store *storage.Storage, schedCfg scheduler.Config) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 30 * time.Second},
	}

	tg, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	menu.Reply(
		menu.Row(btnGround, btnChaos),
		menu.Row(btnRandomize, btnPredict),
	)

	b := &Bot{
		tg:      tg,
		claude:  cl,
		store:   store,
		ownerID: ownerID,
	}

	b.scheduler = scheduler.New(schedCfg, tg, cl, store)
	b.registerHandlers()

	return b, nil
}

func (b *Bot) registerHandlers() {
	b.tg.Handle("/start", b.handleStart)
	b.tg.Handle(&btnGround, b.handleGrounding)
	b.tg.Handle(&btnChaos, b.handleChaos)
	b.tg.Handle(&btnRandomize, b.handleRandomize)
	b.tg.Handle(&btnPredict, b.handlePrediction)
	b.tg.Handle(&btnMoreGround, b.handleGroundingMore)
	b.tg.Handle(&btnMoreChaos, b.handleChaosMore)
	b.tg.Handle("/capsule", b.handleCapsule)
	b.tg.Handle(tele.OnText, b.handleText)
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
