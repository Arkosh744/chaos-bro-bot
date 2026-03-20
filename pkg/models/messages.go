package models

// --- Bot core ---

const (
	MsgStartup      = "Йо, я проснулся. Готов к хаосу."
	MsgMoreFeatures = "🎭 Дополнительные возможности:"
	MsgMainMenu     = "Главное меню:"
)

// --- Button labels ---

const (
	BtnGround    = "👁 Очнись"
	BtnChaos     = "🎲 Ебани куба"
	BtnPredict   = "🔮 Судьба"
	BtnRandomize = "🎱 Кинь кости"
	BtnBreathe   = "🫁 Дыши"
	BtnMore      = "➡️ Ещё"
	BtnRoast     = "🔥 Зажарь"
	BtnWisdom    = "🧙 Мудрость"
	BtnHoroscope = "⭐ Гороскоп"
	BtnMood      = "📊 Настроение"
	BtnMirror    = "🪞 Зеркало"
	BtnBack      = "⬅️ Назад"
)

// --- Inline button labels ---

const (
	BtnMoreGroundLabel  = "🔄 Ещё"
	BtnMoreChaosLabel   = "🔄 Другое"
	BtnReflectGoodLabel = "\U0001F60A Что хорошего"
	BtnReflectBadLabel  = "\U0001F624 Что бесило"
	BtnReflectTmrwLabel = "🎯 Что завтра"
	BtnRepeatLabel      = "\U0001F504 Ещё"
	BtnEditProfileLabel = "\u270F\uFE0F Редактировать"
	BtnHabitDoneLabel   = "\u2705 Сделал"
	BtnHabitSkipLabel   = "\u274C Нет"
)

// --- Game button labels ---

const (
	BtnGameGuess   = "🔢 Угадай число"
	BtnGameTD      = "🤔 Правда/Действие"
	BtnGameDanetki = "🧩 Данетки"
	BtnGameTrivia  = "📚 Тривиа"
	BtnTDTruth     = "🤫 Правда"
	BtnTDDare      = "🎬 Действие"
)

// --- Duel button labels ---

const (
	BtnDuelKnowledge = "\U0001F9E0 Знания"
	BtnDuelHumor     = "\U0001F602 Юмор"
	BtnDuelGames     = "\U0001F3AE Игры"
	BtnDuelAbsurd    = "\U0001F92A Абсурд"
)

// --- Rate limit ---

const MsgRateLimit = "Ты слишком много пишешь. Отдохни часок. \U0001F417"

// --- Achievements ---

const (
	MsgNoAchievements     = "У тебя пока нет ачивок. Давай, начинай играть."
	MsgAchievementsHeader = "\U0001F3C6 Твои ачивки:\n\n"
	MsgAchievementsNext   = "\nБлижайшие:\n"
	FmtAchievementLine    = "%s %s — %s\n"
	FmtAchievementLocked  = "\U0001F512 %s %s (%d/%d)\n"
	FmtAchievementUnlock  = "\U0001F3C6 Ачивка: %s %s — %s"
)

// --- Help ---

const MsgHelp = `🎭 *Команды Трикстера:*

*Кнопки:*
👁 Очнись — техника заземления
🎲 Ебани куба — хаос-задание
🔮 Судьба — предсказание
🎱 Кинь кости — рандомайзер решений
🔥 Зажарь — roast на основе профиля
🧙 Мудрость — абсурдная мудрость
⭐ Гороскоп — антигороскоп дня
📊 Настроение — график за 7 дней
🪞 Зеркало — копирует твой стиль (10 сообщ.)
🫁 Дыши — дыхательный таймер

*Команды:*
/trickster — представиться в группе
/profile — твой профиль
/level — уровень отношений
/achievements — ачивки
/top — таблица лидеров
/silence — режим эмоджи (24ч)
/mirror — зеркало стиля (10 сообщ.)
/meditate — guided медитация
/truth — раскрыть сегодняшнюю ложь бота
/capsule N текст — капсула времени
/remind 30m текст — напоминание
/streak — серия дней подряд
/habit — трекер привычек
/sleep — анализ сна за неделю
/playlist — плейлист под настроение
/future — письмо от тебя из будущего
/challenge — недельный челлендж
/anon текст — анонимное сообщение (группы)
/story — интерактивная история
/game — мини-игры (угадайка, тривиа, данетки)
/voice текст — озвучить текст голосом
/help — эта справка

*Социальные (группы):*
/duel — вызвать на дуэль (ответом на сообщение)
/quest — квест для группы
/link @user — связать аккаунты
/unlink — разорвать связь

*Секретные (streak):*
/roastme — бот roast'ит себя (14д)
/serious — серьёзный ответ (30д)

*Меню:* ➡️ Ещё — доп. кнопки, ⬅️ Назад — главное меню

*Просто пиши* — трикстер ответит с характером`

// --- Start / greeting ---

const (
	FmtStartReturning = "С возвращением! %s %s (ур. %d), streak: %d дн., ачивок: %d"
	FmtStartGreeting  = "Йо. Я *%s*."
	FmtStartAlterEgo  = "Йо. Сегодня я *%s*. Режим: _%s_."
	MsgStartIntro     = "\n\nЯ дерзкий друг-трикстер. Не коуч, не AI, не мамка.\nЖми кнопки или просто пиши мне. /help — все команды."
	MsgStartGrounding = "👁 Вот тебе для начала:\n\n"
)

// --- Grounding / Chaos ---

const (
	MsgGroundPrefix = "👁 "
	MsgChaosPrefix  = "🎲 "
)

// --- Randomizer ---

const MsgRandomizerPrompt = "Окей, кидай вопрос. Я решу за тебя."
const MsgRandomizerPrefix = "🎰 Вижу вопрос — решаю за тебя:\n\n"

// --- Silence mode ---

const (
	FmtSilenceOff     = "\U0001F50A Молчание снято. Оставалось %dч."
	MsgSilenceOn      = "\U0001F92B"
	MsgSilenceFallback = "\U0001F636"
)

// --- Mirror mode ---

const (
	MsgMirrorOff       = "\U0001FA9E Зеркало выключено."
	MsgMirrorOn        = "\U0001FA9E Зеркало активировано. Следующие 10 сообщений я буду говорить как ты."
	MsgMirrorDone      = "\n\n\U0001FA9E Зеркало выключено. Я снова я."
	MsgMirrorHalf      = "\n\n_\U0001FA9E Половина — осталось 5_"
	MsgMirrorLast      = "\n\n_\U0001FA9E Последнее зеркальное сообщение!_"
)

// --- Breathing / Meditate ---

const (
	MsgBreathingStart     = "\U0001FAC1 Приготовься..."
	MsgBreathingFail      = "Не получилось запустить таймер. "
	MsgMeditateStart      = "🧘 Приготовься к медитации..."
	MsgMeditateFail       = "Не получилось. "
)

// --- Capsule ---

const (
	MsgCapsuleFormat      = "Формат: /capsule 7 твоё сообщение\nЧисло = через сколько дней доставить."
	MsgCapsuleFormatShort = "Формат: /capsule 7 привет из прошлого"
	MsgCapsuleDaysRange   = "Дней от 1 до 365. Пример: /capsule 30 привет из прошлого"
	FmtCapsuleSaved       = "\u231B Записал. Доставлю через %d дн. Ты забудешь, а я — нет."
	FmtCapsuleDelivered   = "⏳ Капсула из прошлого:\n\n%s"
)

// --- Mood ---

const (
	MsgMoodNoData  = "Нет данных. Жди утреннего чекина."
	MsgMoodError   = "Не получилось загрузить историю настроения."
	FmtMoodScore   = "Утро. Как ты от 1 до 10?\n\n*%d* — %s"
	MsgMoodHeader  = "Твоё настроение за 7 дней:\n\n"
	MsgMoodMorning = "Утро. Как ты от 1 до 10?"
)

// --- Truth ---

const (
	MsgTruthHonest    = "Сегодня я был честен. Или нет... \U0001F914"
	FmtTruthRevealed  = "Ты уже знаешь. Я соврал: %s\n\nНа самом деле: %s"
	FmtTruthReveal    = "Я соврал: %s\n\nНа самом деле: %s"
)

// --- Profile ---

const (
	MsgProfileEmpty          = "Пока ничего не знаю о тебе. Пиши больше — разберусь."
	MsgProfileHeader         = "📋 *Твой профиль:*\n\n"
	MsgProfileFooter         = "\n_Собрано из наших разговоров. Я внимательный._"
	MsgProfileEditPrompt     = "Напиши факт в формате: категория: значение\nПример: job: Go разработчик\n\nДоступные категории: name, age, city, job, hobbies, music, games, food, relationships, pets, goals, quirks"
	MsgProfileSaveError      = "Не удалось сохранить. Попробуй ещё раз."
	FmtProfileUpdated        = "\u2705 Обновлено: %s: %s"
	MsgProfileFormatError    = "Неверный формат. Используй: категория: значение\nПример: job: Go разработчик"
	MsgProfileNotFilled      = "Профиль пока не заполнен."
)

// --- Remind ---

const (
	MsgRemindFormat  = "Формат: /remind 30m выпить воду\nПоддерживается: Nm (минуты), Nh (часы), Nd (дни)"
	MsgRemindMinTime = "Минимум 1 минута. Не торопись."
	FmtRemindSaved   = "⏰ Запомнил. Напомню через %s."
	FmtRemindDeliver = "⏰ Эй! Ты просил напомнить: %s"
)

// --- Streak ---

const (
	MsgStreakNone = "У тебя пока нет серии. Напиши мне завтра тоже."
	FmtStreak     = "🔥 Твоя серия: %d дней подряд\nРекорд: %d дней"
)

// --- Habit ---

const (
	MsgHabitAddFormat    = "Формат: /habit add название привычки"
	MsgHabitAdded        = "Привычка добавлена. Теперь отмазки не прокатят. /habit done N"
	MsgHabitNotFound     = "Привычка не найдена. /habit list"
	MsgHabitAlreadyDone  = "Уже отмечено. Молодец, но два раза не считается."
	MsgHabitUnknown      = "Непонятно. Попробуй: /habit add, /habit list, /habit done N, /habit delete N"
	MsgHabitNoHabits     = "У тебя нет привычек. Добавь: /habit add пить воду"
	MsgHabitListHeader   = "📋 Твои привычки:\n\n"
	MsgHabitListFooter   = "\n/habit done N — отметить\n/habit add — добавить"
	MsgHabitDeleteFormat = "Формат: /habit delete N (номер привычки из списка)"
	MsgHabitDoneFormat   = "Формат: /habit done N (номер привычки из списка)"
	FmtHabitDeleted      = "Удалено: %s. Одной проблемой меньше."
	FmtHabitStreak       = " 🔥 Серия: %d дней."
	FmtHabitDoneInline   = " \U0001F525 Серия: %d"
	FmtHabitDoneCallback = "\u2705 %s — готово!%s"
	FmtHabitSkipCallback = "\u274C %s — пропущено. Завтра не забудь."
	MsgHabitCBNotFound   = "Привычка не найдена"
	MsgHabitCBAlready    = "Уже отмечено!"
	MsgHabitCBError      = "Ошибка"
)

// HabitDoneReplies are random confirmations when marking a habit as done.
var HabitDoneReplies = []string{
	"Красава. Так держать.",
	"Отмечено. Ты на шаг ближе к нормальному человеку.",
	"Записал. Завтра тоже не забудь.",
	"Ладно, засчитано. Но я слежу.",
	"Готово. Трикстер гордится. Немного.",
}

// --- Photo ---

const MsgPhotoPrefix = "[\U0001F4F7] "

// --- Voice ---

const (
	MsgVoiceNotConfigured  = "Голосовые не настроены. Нужен Groq API ключ."
	MsgVoiceTooLarge       = "Голосовое слишком большое. Максимум 5MB."
	MsgVoiceNotHeard       = "Не расслышал. Попробуй ещё раз или напиши текстом."
	MsgTTSNotInstalled     = "TTS not installed. Need edge-tts: pip install edge-tts"
	MsgTTSDefault          = "Привет, я трикстер. И я умею говорить."
	MsgTTSFail             = "Не получилось озвучить. "
	MsgTTSWarning          = "🔊 _Иногда буду отвечать голосом. Наушники наготове._"
)

// --- Game ---

const (
	MsgGameChoose     = "🎮 Выбирай игру:"
	MsgGameNoActive   = "Нет активной игры."
	MsgGameOver       = "Игра завершена."
	MsgGuessStart     = "🔢 Я загадал число от 1 до 100. У тебя 7 попыток. Пиши число!"
	MsgGuessNaN       = "Напиши число от 1 до 100."
	MsgGuessRange     = "От 1 до 100, дружище."
	FmtGuessCorrect   = "🎉 Угадал за %d попыток! Число было %d."
	FmtGuessLost      = "💀 Попытки кончились! Число было %d."
	FmtGuessHigher    = "⬆️ Больше! (осталось %d попыток)"
	FmtGuessLower     = "⬇️ Меньше! (осталось %d попыток)"
	MsgTDChoose       = "🤔 Правда или действие?"
	MsgTruthPrefix    = "🤫 Правда:\n\n"
	MsgDarePrefix     = "🎬 Действие:\n\n"
	FmtDanetkiGiveUp  = "🧩 Сдаёшься? Ладно.\n\nОтвет: %s"
	MsgDanetkiStart   = "🧩 Данетка:\n\n%s\n\nЗадавай вопросы, на которые можно ответить Да/Нет.\n/game stop — сдаться и узнать ответ."
	MsgDanetkiBroken  = "Что-то пошло не так. Начни новую игру: /game"
	MsgDanetkiBadJudge = "Не смог оценить вопрос. Попробуй другой."
	FmtTriviaScore    = "📚 Счёт: %d\n\n%s"
	FmtTriviaHighscore = "📚 Твой рекорд в тривии: %d"
	FmtTriviaWrong    = "❌ Неправильно! Правильный ответ: %s\n\nТвой счёт: %d"
	MsgTriviaNewRecord = " (новый рекорд! 🎉)"
	MsgTriviaNoActive  = "Нет активной игры тривии"
	MsgTriviaParseFail = "Не получилось придумать вопрос. Попробуй ещё раз."
	MsgDanetkiParseFail = "Не получилось придумать загадку. Попробуй ещё раз."
)

// --- Duel ---

const (
	MsgDuelOnlyGroups     = "Дуэли работают только в группах. Найди себе соперника."
	MsgDuelReplyNeeded    = "Ответь на сообщение того, кого хочешь вызвать на дуэль."
	MsgDuelSelf           = "Дуэль с собой? Ты либо гений, либо одинок."
	MsgDuelWithBot        = "Не-не-не, я судья, а не участник."
	MsgDuelError          = "Что-то пошло не так. Попробуй позже."
	MsgDuelActive         = "В этом чате уже идёт дуэль. Дождись окончания."
	MsgDuelExpired        = "Дуэль устарела. Начни заново /duel."
	MsgDuelCreateFailed   = "Не удалось создать дуэль."
	MsgDuelTimedOut       = "⏰ Время вышло! Дуэль отменена — оба слишком медленные."
	MsgDuelJudgeFailed    = "Не получилось рассудить дуэль. Ничья!"
	MsgDuelRandomJudge    = "Судья не смог решить, победитель выбран случайно."
	FmtDuelChallenge      = "Дуэль! %s vs %s\n\nВыбери категорию:"
	FmtDuelQuestion       = "Duel!\n\n%s vs %s\n\nВопрос: %s\n\nОба пишите ответ прямо в чат. У вас 60 секунд!"
	FmtDuelWaiting        = "%s ответил! Ждём второго участника..."
	FmtDuelResult         = "Результаты дуэли!\n\n%s: %s\n%s: %s\n\nПобедитель: %s\n%s"
	MsgDuelDefaultPlayer1 = "Игрок 1"
	MsgDuelDefaultPlayer2 = "Игрок 2"
)

// --- Quest ---

const (
	MsgQuestOnlyGroups  = "Квесты работают только в группах."
	MsgQuestError       = "Что-то пошло не так."
	MsgQuestActive      = "Квест уже активен! Выполняй: "
	MsgQuestCreateFail  = "Не удалось создать квест."
	FmtQuestStart       = "Квест!\n\n%s\n\nПервый кто выполнит — побеждает. Пишите ответ прямо в чат!"
	FmtQuestComplete    = "Квест выполнен!\n\n%s справился первым!\n\nКвест был: %s"
)

// --- Link ---

const (
	MsgLinkFormatReply    = "Формат: /link (ответом на сообщение) или /link @username"
	MsgLinkFormatUsername = "Формат: /link @username"
	MsgLinkNotFound       = "Не нашёл пользователя. Он должен хотя бы раз написать боту."
	MsgLinkSelf           = "Нельзя связаться с собой. Хотя, понимаю желание."
	MsgLinkAlreadyLinked  = "Ты уже связан с кем-то. Сначала /unlink."
	MsgLinkConfirmFail    = "Не удалось подтвердить связь."
	MsgLinkAlreadySent    = "Ты уже отправил запрос. Жди подтверждения."
	MsgLinkCreateFail     = "Не удалось создать запрос."
	FmtLinkConfirmed      = "Связь с %s установлена!"
	FmtLinkConfirmNotify  = "%s принял твой запрос на связь! Теперь вы связаны."
	FmtLinkRequestSent    = "Запрос на связь отправлен %s. Ждём подтверждения."
	FmtLinkRequestNotify  = "%s хочет связать ваши аккаунты! Напиши /link @%s чтобы подтвердить."
	MsgUnlinkNotLinked    = "Ты ни с кем не связан."
	MsgUnlinkFail         = "Не удалось разорвать связь."
	MsgUnlinkDone         = "Связь разорвана."
	FmtUnlinkNotify       = "%s разорвал связь."
)

// --- Anon ---

const (
	MsgAnonGroupOnly   = "Эта команда работает только в группах."
	MsgAnonFormat      = "Формат: /anon текст сообщения"
	MsgAnonCooldown    = "Подожди 5 минут перед следующим анонимным сообщением."
	MsgAnonPrefix      = "\U0001F3AD Аноним говорит:\n\n"
	FmtAnonRevealed    = "\U0001F3AD Аноним (кажется это %s) говорит:\n\n"
)

// --- Reflection ---

const (
	MsgReflectionSaved       = "\U0001F4DD Записал. Кабан одобряет. \U0001F417"
	MsgReflectGoodPrompt     = "\U0001F60A Что хорошего сегодня было? Напиши:"
	MsgReflectBadPrompt      = "\U0001F624 Что бесило сегодня? Выпусти пар:"
	MsgReflectTomorrowPrompt = "🎯 Что хочешь сделать завтра? Одну вещь:"
)

// --- Story ---

const (
	MsgStoryAbort    = "История прервана. Напиши /story чтобы начать новую."
	MsgStoryEnd      = "📖 %s\n\n🔚 Конец истории."
	MsgStoryPrefix   = "📖 "
	MsgStoryNoActive = "Нет активной истории. /story"
)

// --- Challenge ---

const (
	MsgChallengeNone     = "Нет активного челленджа. Жди понедельник — получишь новый."
	MsgChallengeNoActive = "Нет активного челленджа."
	MsgChallengeDone     = "Ты уже выполнил все 7 дней! Красава."
	FmtChallengeStatus   = "🏋️ Челлендж недели (с %s):\n\n%s\n\nПрогресс: %s (%d/7)\n\n/challenge done — отметить день"
	FmtChallengeComplete = "🎉 Челлендж выполнен! Все 7 дней!\n\n%s\n\n%s"
	FmtChallengeNew      = "🏋️ Челлендж недели:\n\n%s\n\n/challenge — прогресс\n/challenge done — отметить день"
	FmtChallengeReminder = "Не забудь про челлендж: %s\n%s (%d/7) /challenge done"
)

// ChallengeDoneReplies are random confirmations for challenge progress.
var ChallengeDoneReplies = []string{
	"Записал. Так держать.",
	"Ещё один день в копилку.",
	"Кабан одобряет.",
	"Прогресс — это кайф.",
}

// --- Top ---

const (
	MsgTopEmpty  = "Пока никого нет. Ты будешь первым."
	MsgTopHeader = "🏆 Топ:\n\n"
	FmtTopSelf   = "\nТы: #%d из %d"
)

// --- Trickster intro (group) ---

const MsgTricksterGroupOnly = "Эта команда для групп. Добавь меня в группу!"

// --- RoastMe / Serious ---

const (
	FmtRoastMeLocked   = "Эта команда разблокируется на 14-дневном streak. У тебя: "
	FmtSeriousLocked   = "Эта команда разблокируется на 30-дневном streak. У тебя: "
	MsgSeriousUsedToday = "Серьёзный режим уже использован сегодня. Завтра будет ещё один шанс."
	MsgSeriousDefault   = "Скажи что-нибудь серьёзное и мудрое"
)

// --- Mood drop ---

const MsgMoodDrop = "Эй, я заметил что ты как-то сдулся. Если хочешь поговорить — я тут. А нет — ну и ладно. \U0001F417"

// --- Future letter ---

const MsgFuturePrefix = "\U0001F4E8 Письмо из будущего:\n\n"

// --- Scheduler ---

const (
	MsgEveningCheck       = "\U0001F319 Вечерний чек. Выбери:"
	MsgLinkedDefaultName  = "Твой связанный"
	FmtLinkedSameMood     = "Ты и %s оба сегодня на %d/10. Совпадение? Может поговорите?"
	FmtLinkedCloseMood    = "Ты %d/10, а %s — %d/10. Почти на одной волне."
	FmtLinkedDiffMood     = "У тебя %d/10, а у %s — %d/10. Может стоит списаться?"
	FmtNightOwlWarning    = "Ты опять не спишь в %s. Третий раз за неделю. Ложись."
	MsgDigestPrefix       = "📋 Дайджест недели:\n\n"
)

// --- Group interject ---

const FmtGroupInterjectPrompt = "Кто-то в группе написал: \"%s\"\n\nТы подслушал и хочешь вставить свои 5 копеек. Одно короткое предложение. Дерзко и по делу."

// --- Claude client ---

const MsgClaudeDangerousOutput = "Хм, что-то пошло не так. Попробуй ещё раз."

// --- SOS ---

const SOSMessage = "🆘 Стоп. Я вижу что тебе хреново.\n\n" +
	"1. *Дыши*: вдох 4с → задержка 4с → выдох 6с (3 раза)\n" +
	"2. *Заземлись*: назови 5 вещей которые видишь\n" +
	"3. *Вода*: выпей стакан воды прямо сейчас\n\n" +
	"Если совсем плохо — позвони на горячую линию: *8-800-2000-122* (бесплатно, 24/7)\n\n" +
	"Я рядом. Напиши когда отпустит."

// --- Weekday names ---

var WeekdayNamesShort = []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}

// --- Sleep report ---

const (
	MsgSleepNoData       = "Недостаточно данных для анализа сна. Пиши чаще, тогда будет что анализировать."
	MsgSleepHeader       = "😴 Твой сон за последние дни:\n\n"
	FmtSleepAvg          = "\nСреднее: %.1fч\n"
	FmtSleepLatest       = "Позже всего ложишься: %s (%s). Кабан бы не одобрил."
	MsgSleepCritical     = "\n\n⚠️ Ты спишь в среднем меньше 6 часов. Это пиздец. Серьёзно, ложись раньше."
	MsgSleepWarning      = "\n\n⚠️ Меньше 7 часов в среднем. Не критично, но мозг точно не благодарен."
	MsgSleepLateAfter2   = "\n\n🌙 Ложишься после 2 ночи. Попробуй хотя бы в 1. Твой организм скажет спасибо."
	MsgSleepLateAfterMid = "\n\n🌙 Поздно ложишься. Хотя бы до полуночи попробуй — будет легче вставать."
)

// --- Mirror analysis ---

const (
	MsgMirrorNoData  = "Недостаточно данных для анализа стиля. Пиши в своём обычном стиле трикстера."
	MsgStyleVShort   = "Пишет ОЧЕНЬ коротко (1-2 слова)"
	MsgStyleShort    = "Пишет коротко (одно предложение)"
	MsgStyleMedium   = "Пишет средними сообщениями"
	MsgStyleLong     = "Пишет длинными сообщениями"
	MsgStyleCaps     = "Часто использует КАПС"
	MsgStyleNoDots   = "Не ставит точки в конце предложений"
	MsgStyleExclam   = "Часто использует восклицательные знаки!"
	MsgStyleQuestions = "Часто задаёт вопросы"
	MsgStyleEllipsis = "Использует многоточие..."
	MsgStyleEmoji    = "Активно использует эмоджи"
	MsgStyleNoEmoji  = "Не использует эмоджи"
	MsgStyleHeader   = "Характеристики стиля:\n"
	MsgStyleSamples  = "\n\nПримеры сообщений пользователя:\n"
)

// --- Trickster names pool ---

var TricksterNames = []string{
	"Локи", "Гримшоу Пепельный", "Шут Трёхликий", "Морфей с Района",
	"Джинн Кривое Зеркало", "Пак Лунный", "Рейнеке-лис",
	"Коловрат Бессонный", "Чеширский Бродяга", "Робин Безголовый",
	"Ананси Восьмирукий", "Барон Самди", "Койот Пыльный",
	"Гермес Подъездный", "Одиссей Диванный", "Мерлин Бухой",
	"Фенрир Домашний", "Тиль Уленшпигель", "Джокер из Пятёрки",
	"Голлум с Авито", "Добби Свободный", "Геральт из Пятёрочки",
	"Данте с Районной", "Кратос Уставший", "Довакин Ленивый",
}

// --- Category labels ---

var CategoryLabels = map[string]string{
	"name":          "👤 Имя",
	"age":           "🎂 Возраст",
	"city":          "📍 Город",
	"job":           "💼 Работа",
	"hobbies":       "🎮 Хобби",
	"music":         "🎵 Музыка",
	"games":         "🕹 Игры",
	"food":          "🍕 Еда",
	"mood_pattern":  "😤 Настроение",
	"relationships": "💑 Отношения",
	"pets":          "🐾 Питомцы",
	"goals":         "🎯 Цели",
	"quirks":        "🤪 Странности",
}
