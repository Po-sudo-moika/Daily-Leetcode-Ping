package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/labstack/gommon/log"
)

const (
	callbackSubscribe   = "subscribe"
	callbackUnsubscribe = "unsubscribe"
	callbackAbout       = "about"
	callbackSettings    = "settings"
	callbackMenu        = "menu"
	// callbackHourPrefix is followed by the Moscow hour, like "hour:9". It
	// opens the minutes of that hour and saves nothing.
	callbackHourPrefix = "hour:"
	// callbackTimePrefix is followed by the delivery time in minutes since
	// midnight, like "time:570" for 09:30. It saves the time.
	callbackTimePrefix = "time:"
	// callbackLanguagePrefix is followed by the language to switch to, like
	// "lang:en". It saves the language.
	callbackLanguagePrefix = "lang:"

	// RepositoryURL is the public code of the project.
	RepositoryURL = "https://github.com/Po-sudo-moika/Daily-Leetcode-Ping"

	hoursPerRow   = 4
	minutesPerRow = 4
)

// The texts the menu shows, in each language of a chat.
var (
	welcomeText = message{
		ru: "👋 Привет! Я присылаю задачу дня с LeetCode каждый день, по умолчанию в 11:00 по Москве. " +
			"Время можно выбрать в настройках.",
		en: "👋 Hi! I send the LeetCode problem of the day every day, at 11:00 Moscow time by default. " +
			"You can pick the time in the settings.",
	}

	// greetingText is the hello a chat gets once, before the menu, on its
	// first /start.
	greetingText = message{
		ru: "Привет! Я Daily LeetCode Ping. Каждый день присылаю задачу дня с LeetCode: название, сложность и ссылку. " +
			"Время можно выбрать в настройках, а язык — кнопкой в меню.",
		en: "Hi! I'm Daily LeetCode Ping. Every day I send the LeetCode problem of the day: its title, difficulty and a link. " +
			"You can pick the time in the settings and the language with the button in the menu.",
	}

	chooseText = message{ru: "Выбери действие:", en: "Choose an action:"}

	notSubscribedStatus = message{ru: "🛑 Сейчас подписка отключена.", en: "🛑 The subscription is off now."}

	unsubscribedText = message{
		ru: "🛑 Подписка отключена. Чтобы вернуться, нажми «Подписаться».",
		en: "🛑 The subscription is off. To come back, press “Subscribe”.",
	}

	saveFailedText = message{
		ru: "⚠️ Не получилось сохранить выбор. Попробуй ещё раз чуть позже.",
		en: "⚠️ Could not save your choice. Please try again a little later.",
	}

	// adminsOnlyText is the note a member who is not an admin gets in a group
	// when pressing a button that changes the subscription, the time or the
	// language.
	adminsOnlyText = message{
		ru: "Менять это в группе могут только админы.",
		en: "Only admins can change this in a group.",
	}

	adminCheckFailedText = message{
		ru: "Не получилось проверить, админ ли ты. Попробуй ещё раз чуть позже.",
		en: "Could not check whether you are an admin. Please try again a little later.",
	}

	aboutText = message{
		ru: "ℹ️ Daily LeetCode Ping — небольшой бот, который каждый день в выбранное время " +
			"(по умолчанию в 11:00 МСК) присылает задачу дня с leetcode.com: название, сложность и ссылку.\n\n" +
			"Код открыт — смотри на GitHub.",
		en: "ℹ️ Daily LeetCode Ping is a small bot that sends the daily problem from leetcode.com every day " +
			"at the time you pick (11:00 Moscow time by default): its title, difficulty and a link.\n\n" +
			"The code is open — see it on GitHub.",
	}

	settingsNotSubscribedText = message{
		ru: "⚙️ Настройки\n\nВремя рассылки можно выбрать после подписки. Нажми «Подписаться».",
		en: "⚙️ Settings\n\nYou can pick the time after you subscribe. Press “Subscribe”.",
	}

	subscribeButton   = message{ru: "✅ Подписаться", en: "✅ Subscribe"}
	unsubscribeButton = message{ru: "🛑 Отписаться", en: "🛑 Unsubscribe"}
	settingsButton    = message{ru: "⚙️ Настройки", en: "⚙️ Settings"}
	aboutButton       = message{ru: "ℹ️ О проекте", en: "ℹ️ About"}
	backLabel         = message{ru: "⬅️ Назад", en: "⬅️ Back"}
	codeButton        = message{ru: "📦 Код на GitHub", en: "📦 Code on GitHub"}
)

// languageLabel is the label of the button that switches to lang. It is in
// that language, so the person can read it whatever the chat speaks now.
func languageLabel(lang subscribers.Language) string {
	if lang == subscribers.English {
		return "🇬🇧 English"
	}

	return "🇷🇺 Русский"
}

// What people see in the bot's profile before they press Start. Telegram
// limits: description 512 characters, short description 120. It is the same for
// everyone, in Russian: a per-language profile is a later change.
const (
	botDescription = "Присылаю задачу дня с LeetCode каждый день в удобное время (по умолчанию в 11:00 по Москве): " +
		"название, сложность и ссылку.\n\nНажми «Начать» — подписка включится сама, откроется меню: " +
		"там можно выбрать время рассылки и отписаться. Меня можно добавить и в группу или канал."
	botShortDescription = "Задача дня с LeetCode каждый день в удобное время по Москве"
)

// screen is what the one menu message shows: its text and its buttons.
type screen struct {
	text   string
	markup *models.InlineKeyboardMarkup
}

// target is the menu message a button press came from.
type target struct {
	chatID    int64
	messageID int
}

// Menu answers /start, /settings and the buttons of the inline menu. There is
// one menu message per chat: a button press edits it in place.
type Menu struct {
	api         API
	subscribers Subscribers
	fixedChats  []string
	// ownerID is the Telegram user id of the bot's owner; 0 means /chats
	// answers nobody.
	ownerID int64
	// greetMu makes checking, sending and recording the hello one step. The
	// bot runs handlers at the same time, so two /start from a new chat that
	// arrive together would otherwise both send it.
	greetMu sync.Mutex
}

// NewMenu returns the menu. fixedChats are the chats from config.toml, as
// ResolveFixedChats returns them.
func NewMenu(api API, subscribers Subscribers, fixedChats []string) *Menu {
	return &Menu{api: api, subscribers: subscribers, fixedChats: fixedChats}
}

// WithOwner sets the Telegram user id of the bot's owner, the only person
// /chats answers. It returns the menu.
func (m *Menu) WithOwner(userID int64) *Menu {
	m.ownerID = userID

	return m
}

// Register wires the menu handlers into the bot.
func (m *Menu) Register(b *bot.Bot) {
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return update.MyChatMember != nil }, m.HandleMyChatMember)
	b.RegisterHandler(bot.HandlerTypeMessageText, "start", bot.MatchTypeCommandStartOnly, m.HandleStart)
	b.RegisterHandler(bot.HandlerTypeMessageText, "settings", bot.MatchTypeCommandStartOnly, m.HandleSettingsCommand)
	b.RegisterHandler(bot.HandlerTypeMessageText, "chats", bot.MatchTypeCommandStartOnly, m.HandleChats)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackSubscribe, bot.MatchTypeExact, m.HandleSubscribe)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackUnsubscribe, bot.MatchTypeExact, m.HandleUnsubscribe)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackAbout, bot.MatchTypeExact, m.HandleAbout)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackSettings, bot.MatchTypeExact, m.HandleSettings)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackMenu, bot.MatchTypeExact, m.HandleBack)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackHourPrefix, bot.MatchTypePrefix, m.HandleHour)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackTimePrefix, bot.MatchTypePrefix, m.HandleTime)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackLanguagePrefix, bot.MatchTypePrefix, m.HandleLanguage)
}

// Setup publishes the bot's profile: the text shown before Start and the
// command list behind the chat's Menu button. A failed call does not stop the
// others; the errors are joined.
func (m *Menu) Setup(ctx context.Context) error {
	var errs []error

	_, err := m.api.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: []models.BotCommand{
		{Command: "start", Description: "Открыть меню"},
		{Command: "settings", Description: "Настройки: время рассылки"},
	}})
	if err != nil {
		errs = append(errs, fmt.Errorf("set commands: %w", err))
	}

	if _, err := m.api.SetMyDescription(ctx, &bot.SetMyDescriptionParams{Description: botDescription}); err != nil {
		errs = append(errs, fmt.Errorf("set description: %w", err))
	}

	if _, err := m.api.SetMyShortDescription(ctx, &bot.SetMyShortDescriptionParams{ShortDescription: botShortDescription}); err != nil {
		errs = append(errs, fmt.Errorf("set short description: %w", err))
	}

	return errors.Join(errs...)
}

// HandleStart opens the menu in a new message. Every button press after it
// edits that message. The first time a chat starts the bot, a short hello comes
// before the menu.
func (m *Menu) HandleStart(ctx context.Context, _ *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	m.greet(ctx, chatID)
	m.send(ctx, chatID, m.welcomeScreen(chatID))
}

// greet sends the hello to a chat that has not had it yet, and records that it
// has. A hello that could not be sent is not recorded, so the next /start tries
// again.
func (m *Menu) greet(ctx context.Context, chatID int64) {
	m.greetMu.Lock()
	defer m.greetMu.Unlock()

	if m.subscribers.Greeted(chatID) {
		return
	}

	text := greetingText.in(m.subscribers.Language(chatID))
	if _, err := m.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text}); err != nil {
		log.Errorf("error while sending the greeting to %d: %v", chatID, err)
		return
	}

	if err := m.subscribers.MarkGreeted(chatID); err != nil {
		log.Errorf("error while recording the greeting of %d: %v", chatID, err)
	}
}

// HandleSettingsCommand opens the settings screen in a new message.
func (m *Menu) HandleSettingsCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	m.send(ctx, chatID, m.settingsScreen(chatID, ""))
}

func (m *Menu) HandleSubscribe(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, adminOnly)
	if !ok {
		return
	}

	lang := m.subscribers.Language(t.chatID)
	if _, err := m.subscribers.Add(t.chatID); err != nil {
		log.Errorf("error while changing subscription of %d: %v", t.chatID, err)
		m.show(ctx, t, mainScreen(saveFailedText.in(lang), lang))
		return
	}

	at, _ := m.subscribers.Time(t.chatID)
	m.show(ctx, t, mainScreen(subscribedText(at, lang), lang))
}

func (m *Menu) HandleUnsubscribe(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, adminOnly)
	if !ok {
		return
	}

	lang := m.subscribers.Language(t.chatID)
	if _, err := m.subscribers.Remove(t.chatID); err != nil {
		log.Errorf("error while changing subscription of %d: %v", t.chatID, err)
		m.show(ctx, t, mainScreen(saveFailedText.in(lang), lang))
		return
	}

	text := unsubscribedText.in(lang)
	if m.isFixed(t.chatID) {
		text = fixedStatus(lang)
	}
	m.show(ctx, t, mainScreen(text, lang))
}

func (m *Menu) HandleAbout(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, anyone)
	if !ok {
		return
	}

	m.show(ctx, t, aboutScreen(m.subscribers.Language(t.chatID)))
}

func (m *Menu) HandleSettings(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, anyone)
	if !ok {
		return
	}

	m.show(ctx, t, m.settingsScreen(t.chatID, ""))
}

// HandleBack returns from the about or settings screen to the menu.
func (m *Menu) HandleBack(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, anyone)
	if !ok {
		return
	}

	m.show(ctx, t, m.welcomeScreen(t.chatID))
}

// HandleHour shows the minutes of the pressed hour. Nothing is saved until a
// minute is pressed.
func (m *Menu) HandleHour(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, adminOnly)
	if !ok {
		return
	}

	hour, err := strconv.Atoi(strings.TrimPrefix(update.CallbackQuery.Data, callbackHourPrefix))
	if _, subscribed := m.subscribers.Time(t.chatID); err != nil || !subscribed || hour < subscribers.MinHour || hour > subscribers.MaxHour {
		// A stale or made-up button: show the screen as it really is.
		m.show(ctx, t, m.settingsScreen(t.chatID, ""))
		return
	}

	m.show(ctx, t, m.minutesScreen(t.chatID, hour))
}

// HandleTime saves the delivery time of the pressed button and shows the
// settings screen again with the new choice marked.
func (m *Menu) HandleTime(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, adminOnly)
	if !ok {
		return
	}

	lang := m.subscribers.Language(t.chatID)
	at, err := strconv.Atoi(strings.TrimPrefix(update.CallbackQuery.Data, callbackTimePrefix))
	if err == nil {
		err = m.subscribers.SetTime(t.chatID, at)
	}

	switch {
	case err == nil:
		saved := fmt.Sprintf(message{
			ru: "✅ Сохранено: каждый день в %s (МСК).",
			en: "✅ Saved: every day at %s (Moscow time).",
		}.in(lang), subscribers.Format(at))
		m.show(ctx, t, m.settingsScreen(t.chatID, saved))
	case errors.Is(err, subscribers.ErrNotSubscribed), errors.Is(err, subscribers.ErrInvalidTime), isBadNumber(err):
		// A stale or made-up button: show the screen as it really is.
		m.show(ctx, t, m.settingsScreen(t.chatID, ""))
	default:
		log.Errorf("error while saving time of %d: %v", t.chatID, err)
		m.show(ctx, t, m.settingsScreen(t.chatID, saveFailedText.in(lang)))
	}
}

// HandleLanguage saves the language of the pressed button for the chat and
// shows the menu in it. The button names the language to switch to, so two
// presses of the same button never switch back.
func (m *Menu) HandleLanguage(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, adminOnly)
	if !ok {
		return
	}

	lang := subscribers.Language(strings.TrimPrefix(update.CallbackQuery.Data, callbackLanguagePrefix))
	if lang != subscribers.Russian && lang != subscribers.English {
		// A stale or made-up button: show the screen as it really is.
		m.show(ctx, t, m.welcomeScreen(t.chatID))
		return
	}

	if err := m.subscribers.SetLanguage(t.chatID, lang); err != nil {
		log.Errorf("error while saving language of %d: %v", t.chatID, err)
		current := m.subscribers.Language(t.chatID)
		m.show(ctx, t, mainScreen(saveFailedText.in(current), current))
		return
	}

	m.show(ctx, t, m.welcomeScreen(t.chatID))
}

// isBadNumber reports a callback whose value is not a number.
func isBadNumber(err error) bool {
	var numErr *strconv.NumError

	return errors.As(err, &numErr)
}

// Who may press a button: anyone, or only the admins of a group.
const (
	anyone    = false
	adminOnly = true
)

// ackCallback answers the button press, which stops the spinner, and returns
// the menu message of the pressed button. It reports false when the update has
// no usable chat, or when the button is adminOnly, the chat is a group or a
// supergroup and the person who pressed it is not an admin there: that person
// gets a short note instead, and nothing changes. Private chats and channels
// have no such limit.
func (m *Menu) ackCallback(ctx context.Context, update *models.Update, only bool) (target, bool) {
	query := update.CallbackQuery
	if query == nil {
		return target{}, false
	}

	// Telegram sends an inaccessible message when the menu message is too old.
	// It still carries the chat and the message id, so the buttons keep
	// working: show tries the edit and falls back to a new menu.
	var t target
	var chatType models.ChatType
	found := true
	switch {
	case query.Message.Message != nil:
		t = target{chatID: query.Message.Message.Chat.ID, messageID: query.Message.Message.ID}
		chatType = query.Message.Message.Chat.Type
	case query.Message.InaccessibleMessage != nil:
		t = target{chatID: query.Message.InaccessibleMessage.Chat.ID, messageID: query.Message.InaccessibleMessage.MessageID}
		chatType = query.Message.InaccessibleMessage.Chat.Type
	default:
		found = false
	}

	note := ""
	if found && only && (chatType == models.ChatTypeGroup || chatType == models.ChatTypeSupergroup) {
		note = m.adminNote(ctx, t.chatID, query.From.ID, m.subscribers.Language(t.chatID))
	}

	if _, err := m.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: note}); err != nil {
		log.Errorf("error while answering callback query: %v", err)
	}

	return t, found && note == ""
}

// adminNote returns "" when the user is an admin (creator or administrator) of
// the group, and otherwise the note to show. When Telegram cannot say, the user
// is not let through: nothing changes, and they are asked to try again.
func (m *Menu) adminNote(ctx context.Context, chatID, userID int64, lang subscribers.Language) string {
	member, err := m.api.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		log.Errorf("error while checking whether user %d is an admin of chat %d: %v", userID, chatID, err)
		return adminCheckFailedText.in(lang)
	}

	switch member.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator:
		return ""
	}

	return adminsOnlyText.in(lang)
}

// show puts the screen into the menu message, in place. If Telegram refuses
// the edit because the message is gone or too old, it sends one new menu.
func (m *Menu) show(ctx context.Context, t target, s screen) {
	_, err := m.api.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      t.chatID,
		MessageID:   t.messageID,
		Text:        s.text,
		ReplyMarkup: s.markup,
	})

	switch {
	case err == nil, isNotModified(err):
		// Pressing a button that changes nothing is fine.
	case errors.Is(err, bot.ErrorBadRequest):
		log.Infof("menu message %d of chat %d can't be edited, sending a new one: %v", t.messageID, t.chatID, err)
		m.send(ctx, t.chatID, s)
	default:
		log.Errorf("error while editing menu message %d of chat %d: %v", t.messageID, t.chatID, err)
	}
}

func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}

func (m *Menu) send(ctx context.Context, chatID int64, s screen) {
	_, err := m.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        s.text,
		ReplyMarkup: s.markup,
	})
	if err != nil {
		log.Errorf("error while sending menu message to %d: %v", chatID, err)
	}
}

func (m *Menu) settingsScreen(chatID int64, note string) screen {
	lang := m.subscribers.Language(chatID)
	at, subscribed := m.subscribers.Time(chatID)
	if !subscribed {
		return screen{
			text: withNote(note, settingsNotSubscribedText.in(lang)),
			markup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: subscribeButton.in(lang), CallbackData: callbackSubscribe}},
				{backButton(lang)},
			}},
		}
	}

	text := fmt.Sprintf(message{
		ru: "⚙️ Настройки\n\nВо сколько присылать задачу дня? Время московское (МСК, UTC+3). " +
			"Сейчас: %s.\n\nСначала выбери час, потом минуты (с шагом %d минут). " +
			"Задача на LeetCode меняется в 03:00 МСК, поэтому доступно время с %s до %s.",
		en: "⚙️ Settings\n\nAt what time should I send the problem of the day? The time is Moscow time (MSK, UTC+3). " +
			"Now: %s.\n\nFirst pick the hour, then the minutes (in steps of %d minutes). " +
			"The LeetCode problem changes at 03:00 MSK, so the times from %s to %s are available.",
	}.in(lang), subscribers.Format(at), subscribers.MinuteStep, subscribers.Format(subscribers.MinTime), subscribers.Format(subscribers.MaxTime))

	var rows [][]models.InlineKeyboardButton
	for h := subscribers.MinHour; h <= subscribers.MaxHour; h++ {
		if (h-subscribers.MinHour)%hoursPerRow == 0 {
			rows = append(rows, nil)
		}

		label := hourLabel(h)
		if h == at/60 {
			label = "✅ " + subscribers.Format(at)
		}
		last := len(rows) - 1
		rows[last] = append(rows[last], models.InlineKeyboardButton{Text: label, CallbackData: callbackHourPrefix + strconv.Itoa(h)})
	}
	rows = append(rows, []models.InlineKeyboardButton{backButton(lang)})

	return screen{text: withNote(note, text), markup: &models.InlineKeyboardMarkup{InlineKeyboard: rows}}
}

// minutesScreen offers the minutes of one hour, with the current choice marked
// when it falls in that hour. Its Back button returns to the hours.
func (m *Menu) minutesScreen(chatID int64, hour int) screen {
	lang := m.subscribers.Language(chatID)
	at, _ := m.subscribers.Time(chatID)

	text := fmt.Sprintf(message{
		ru: "⚙️ Настройки\n\nВыбери минуты для %02d:__ (МСК). Сейчас: %s.",
		en: "⚙️ Settings\n\nPick the minutes for %02d:__ (MSK). Now: %s.",
	}.in(lang), hour, subscribers.Format(at))

	var rows [][]models.InlineKeyboardButton
	for minute := 0; minute < 60; minute += subscribers.MinuteStep {
		if (minute/subscribers.MinuteStep)%minutesPerRow == 0 {
			rows = append(rows, nil)
		}

		choice := subscribers.At(hour, minute)
		label := subscribers.Format(choice)
		if choice == at {
			label = "✅ " + label
		}
		last := len(rows) - 1
		rows[last] = append(rows[last], models.InlineKeyboardButton{Text: label, CallbackData: callbackTimePrefix + strconv.Itoa(choice)})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: backLabel.in(lang), CallbackData: callbackSettings}})

	return screen{text: text, markup: &models.InlineKeyboardMarkup{InlineKeyboard: rows}}
}

func withNote(note, text string) string {
	if note == "" {
		return text
	}

	return note + "\n\n" + text
}

// welcomeScreen is the menu with the chat's subscription state. A fixed chat
// without a subscription of its own still gets the post, and is told so.
func (m *Menu) welcomeScreen(chatID int64) screen {
	lang := m.subscribers.Language(chatID)
	at, subscribed := m.subscribers.Time(chatID)
	if !subscribed && m.isFixed(chatID) {
		return mainScreen(welcomeWith(fixedStatus(lang), lang), lang)
	}

	return mainScreen(welcomeFor(at, subscribed, lang), lang)
}

// isFixed reports whether the chat is one of the chats from config.toml.
func (m *Menu) isFixed(chatID int64) bool {
	for _, chat := range m.fixedChats {
		if id, err := strconv.ParseInt(chat, 10, 64); err == nil && id == chatID {
			return true
		}
	}

	return false
}

// welcomeFor is the menu text for a chat that is subscribed at time at, or is
// not subscribed.
func welcomeFor(at int, subscribed bool, lang subscribers.Language) string {
	status := notSubscribedStatus.in(lang)
	if subscribed {
		status = fmt.Sprintf(message{
			ru: "✅ Сейчас подписка включена: каждый день в %s (МСК).",
			en: "✅ The subscription is on: every day at %s (Moscow time).",
		}.in(lang), subscribers.Format(at))
	}

	return welcomeWith(status, lang)
}

func welcomeWith(status string, lang subscribers.Language) string {
	return welcomeText.in(lang) + "\n\n" + status + "\n\n" + chooseText.in(lang)
}

// fixedStatus is the state of a fixed chat without a subscription of its own:
// it gets the post anyway, at the fixed time.
func fixedStatus(lang subscribers.Language) string {
	return fmt.Sprintf(message{
		ru: "✅ Этот чат в списке рассылки бота: задача дня приходит сюда каждый день в %s (МСК).",
		en: "✅ This chat is on the bot's mailing list: the problem of the day comes here every day at %s (Moscow time).",
	}.in(lang), subscribers.Format(fixedChatsTime))
}

func subscribedText(at int, lang subscribers.Language) string {
	return fmt.Sprintf(message{
		ru: "✅ Подписка включена. Задача дня будет приходить каждый день в %s (МСК). " +
			"Время можно поменять в настройках.",
		en: "✅ The subscription is on. The problem of the day will come every day at %s (Moscow time). " +
			"You can change the time in the settings.",
	}.in(lang), subscribers.Format(at))
}

// hourLabel is the label of an hour button, like "09:00".
func hourLabel(hour int) string {
	return subscribers.Format(subscribers.At(hour, 0))
}

func backButton(lang subscribers.Language) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: backLabel.in(lang), CallbackData: callbackMenu}
}

// mainScreen is the menu in lang. Its last button switches to the other
// language and is labelled in it.
func mainScreen(text string, lang subscribers.Language) screen {
	other := lang.Other()

	return screen{
		text: text,
		markup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: subscribeButton.in(lang), CallbackData: callbackSubscribe},
					{Text: unsubscribeButton.in(lang), CallbackData: callbackUnsubscribe},
				},
				{
					{Text: settingsButton.in(lang), CallbackData: callbackSettings},
					{Text: aboutButton.in(lang), CallbackData: callbackAbout},
				},
				{
					{Text: languageLabel(other), CallbackData: callbackLanguagePrefix + string(other)},
				},
			},
		},
	}
}

func aboutScreen(lang subscribers.Language) screen {
	return screen{
		text: aboutText.in(lang),
		markup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: codeButton.in(lang), URL: RepositoryURL}},
				{backButton(lang)},
			},
		},
	}
}
