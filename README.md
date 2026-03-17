# 🎭 Trickster Bot

Дерзкий друг-трикстер в Telegram. Помогает жить в моменте через сарказм, абсурд и хаос. Не коуч, не терапевт, не AI-ассистент.

Работает через `claude -p` (Claude Code CLI) — ноль внешних API ключей.

## Фичи

| Кнопка | Что делает |
|--------|-----------|
| **👁 Очнись** | Grounding-техника: дыхание, 5 чувств, наблюдение. 30+ упражнений |
| **🎲 Ебани куба** | Рандомное микро-задание. 47 заготовок + генерация через Claude |
| **🎱 Кинь кости** | Задай вопрос — бот решит за тебя. Однозначно и дерзко |
| **Просто текст** | Свободный чат с трикстером. Помнит контекст |

**Память** — хранит все разговоры в SQLite, сжимает в summary каждые 20 сообщений. Замечает паттерны ("опять ноешь про работу").

**Рандомные пинги** — бот сам пишет 2-4 раза в день: цитаты из Warcraft/StarCraft/Diablo, grounding или просто подъёбка с контекстом.

**Thinking-анимация** — 🤔 → 🤔. → 🤔.. → ответ. Пока Claude думает.

**Рандомная личность** — каждый /start новое имя: "Геральт из Пятёрочки", "Мерлин Бухой", "Голлум с Авито".

## Быстрый старт

```bash
git clone https://github.com/Arkosh744/chaos-bro-bot.git
cd trickster-bot

go mod tidy

cp config.example.yaml config.local.yaml
# Вставь токен бота и свой Telegram ID в config.local.yaml

go build -o bot ./cmd/bot/
./bot
```

### Требования

- Go 1.22+
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) (`claude` в PATH)
- Telegram Bot Token ([@BotFather](https://t.me/BotFather))

## Как работает

```
Сообщение → SQLite → summary + last 5 → claude -p → ответ → SQLite
                                                         ↓
                                    каждые ~20 сообщений → обновляет summary
```

## Структура

```
cmd/bot/main.go              — entrypoint
internal/
├── bot/                      — telebot, handlers, thinking-анимация
├── claude/                   — обёртка claude -p
├── config/                   — YAML + env expansion
├── features/                 — prompts, grounding, chaos, quotes, memory, fallback
├── scheduler/                — рандомные пинги 2-6ч
└── storage/                  — SQLite: messages + context_summary
```

## Тесты

```bash
# Unit (быстро)
go test ./internal/storage/ ./internal/config/ ./internal/features/ \
  -run 'TestStorage|TestLoad|TestBuildContext|TestRandom'

# Интеграционные (нужен claude CLI, ~2 мин)
go test -v -timeout=600s ./... -run 'Integration'

# Всё
go test -v -timeout=600s ./...
```

## License

MIT
