package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
	// callbackHourPrefix is followed by the Moscow hour, like "hour:9".
	callbackHourPrefix = "hour:"

	// RepositoryURL is the public code of the project.
	RepositoryURL = "https://github.com/Po-sudo-moika/Daily-Leetcode-Ping"

	welcomeText = "👋 Привет! Я присылаю задачу дня с LeetCode каждый день, по умолчанию в 11:00 по Москве. " +
		"Время можно выбрать в настройках."

	chooseText = "Выбери действие:"

	notSubscribedStatus = "🛑 Сейчас подписка отключена."

	unsubscribedText = "🛑 Подписка отключена. Чтобы вернуться, нажми «Подписаться»."

	saveFailedText = "⚠️ Не получилось сохранить выбор. Попробуй ещё раз чуть позже."

	aboutText = "ℹ️ Daily LeetCode Ping — небольшой бот, который каждый день в выбранное время " +
		"(по умолчанию в 11:00 МСК) присылает задачу дня с leetcode.com: название, сложность и ссылку.\n\n" +
		"Код открыт — смотри на GitHub."

	settingsNotSubscribedText = "⚙️ Настройки\n\nВремя рассылки можно выбрать после подписки. Нажми «Подписаться»."

	// What people see in the bot's profile before they press Start. Telegram
	// limits: description 512 characters, short description 120.
	botDescription = "Присылаю задачу дня с LeetCode каждый день в удобный час (по умолчанию в 11:00 по Москве): " +
		"название, сложность и ссылку.\n\nНажми «Начать» — подписка включится сама, откроется меню: " +
		"там можно выбрать час рассылки и отписаться. Меня можно добавить и в группу или канал."
	botShortDescription = "Задача дня с LeetCode каждый день в удобный час по Москве"

	hoursPerRow = 4
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
}

func NewMenu(api API, subscribers Subscribers) *Menu {
	return &Menu{api: api, subscribers: subscribers}
}

// Register wires the menu handlers into the bot.
func (m *Menu) Register(b *bot.Bot) {
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return update.MyChatMember != nil }, m.HandleMyChatMember)
	b.RegisterHandler(bot.HandlerTypeMessageText, "start", bot.MatchTypeCommandStartOnly, m.HandleStart)
	b.RegisterHandler(bot.HandlerTypeMessageText, "settings", bot.MatchTypeCommandStartOnly, m.HandleSettingsCommand)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackSubscribe, bot.MatchTypeExact, m.HandleSubscribe)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackUnsubscribe, bot.MatchTypeExact, m.HandleUnsubscribe)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackAbout, bot.MatchTypeExact, m.HandleAbout)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackSettings, bot.MatchTypeExact, m.HandleSettings)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackMenu, bot.MatchTypeExact, m.HandleBack)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackHourPrefix, bot.MatchTypePrefix, m.HandleHour)
}

// Setup publishes the bot's profile: the text shown before Start and the
// command list behind the chat's Menu button. A failed call does not stop the
// others; the errors are joined.
func (m *Menu) Setup(ctx context.Context) error {
	var errs []error

	_, err := m.api.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: []models.BotCommand{
		{Command: "start", Description: "Открыть меню"},
		{Command: "settings", Description: "Настройки: час рассылки"},
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
// edits that message.
func (m *Menu) HandleStart(ctx context.Context, _ *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	m.send(ctx, chatID, m.welcomeScreen(chatID))
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
	t, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	if _, err := m.subscribers.Add(t.chatID); err != nil {
		log.Errorf("error while changing subscription of %d: %v", t.chatID, err)
		m.show(ctx, t, mainScreen(saveFailedText))
		return
	}

	hour, _ := m.subscribers.Hour(t.chatID)
	m.show(ctx, t, mainScreen(subscribedText(hour)))
}

func (m *Menu) HandleUnsubscribe(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	if _, err := m.subscribers.Remove(t.chatID); err != nil {
		log.Errorf("error while changing subscription of %d: %v", t.chatID, err)
		m.show(ctx, t, mainScreen(saveFailedText))
		return
	}

	m.show(ctx, t, mainScreen(unsubscribedText))
}

func (m *Menu) HandleAbout(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	m.show(ctx, t, aboutScreen())
}

func (m *Menu) HandleSettings(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	m.show(ctx, t, m.settingsScreen(t.chatID, ""))
}

// HandleBack returns from the about or settings screen to the menu.
func (m *Menu) HandleBack(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	m.show(ctx, t, m.welcomeScreen(t.chatID))
}

// HandleHour saves the delivery hour of the pressed button and shows the
// settings screen again with the new choice marked.
func (m *Menu) HandleHour(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	hour, err := strconv.Atoi(strings.TrimPrefix(update.CallbackQuery.Data, callbackHourPrefix))
	if err == nil {
		err = m.subscribers.SetHour(t.chatID, hour)
	}

	switch {
	case err == nil:
		m.show(ctx, t, m.settingsScreen(t.chatID, "✅ Сохранено: каждый день в "+hourLabel(hour)+" (МСК)."))
	case errors.Is(err, subscribers.ErrNotSubscribed), errors.Is(err, subscribers.ErrInvalidHour), isBadHour(err):
		// A stale or made-up button: show the screen as it really is.
		m.show(ctx, t, m.settingsScreen(t.chatID, ""))
	default:
		log.Errorf("error while saving hour of %d: %v", t.chatID, err)
		m.show(ctx, t, m.settingsScreen(t.chatID, saveFailedText))
	}
}

// isBadHour reports a callback whose hour is not a number.
func isBadHour(err error) bool {
	var numErr *strconv.NumError

	return errors.As(err, &numErr)
}

// ackCallback stops the button spinner and returns the menu message of the
// pressed button. It reports false when the update has no usable chat.
func (m *Menu) ackCallback(ctx context.Context, update *models.Update) (target, bool) {
	query := update.CallbackQuery
	if query == nil {
		return target{}, false
	}

	if _, err := m.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
		log.Errorf("error while answering callback query: %v", err)
	}

	// Telegram sends an inaccessible message when the menu message is too old.
	// It still carries the chat and the message id, so the buttons keep
	// working: show tries the edit and falls back to a new menu.
	switch {
	case query.Message.Message != nil:
		return target{chatID: query.Message.Message.Chat.ID, messageID: query.Message.Message.ID}, true
	case query.Message.InaccessibleMessage != nil:
		return target{chatID: query.Message.InaccessibleMessage.Chat.ID, messageID: query.Message.InaccessibleMessage.MessageID}, true
	}

	return target{}, false
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
		// Pressing a button that changes nothing (the same hour again) is fine.
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
	hour, subscribed := m.subscribers.Hour(chatID)
	if !subscribed {
		return screen{
			text: withNote(note, settingsNotSubscribedText),
			markup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "✅ Подписаться", CallbackData: callbackSubscribe}},
				{backButton()},
			}},
		}
	}

	text := fmt.Sprintf("⚙️ Настройки\n\nВо сколько присылать задачу дня? Время московское (МСК, UTC+3). "+
		"Сейчас: %s.\n\nЗадача на LeetCode меняется в 03:00 МСК, поэтому доступны часы с %s до %s.",
		hourLabel(hour), hourLabel(subscribers.MinHour), hourLabel(subscribers.MaxHour))

	var rows [][]models.InlineKeyboardButton
	for h := subscribers.MinHour; h <= subscribers.MaxHour; h++ {
		if (h-subscribers.MinHour)%hoursPerRow == 0 {
			rows = append(rows, nil)
		}

		label := hourLabel(h)
		if h == hour {
			label = "✅ " + label
		}
		last := len(rows) - 1
		rows[last] = append(rows[last], models.InlineKeyboardButton{Text: label, CallbackData: callbackHourPrefix + strconv.Itoa(h)})
	}
	rows = append(rows, []models.InlineKeyboardButton{backButton()})

	return screen{text: withNote(note, text), markup: &models.InlineKeyboardMarkup{InlineKeyboard: rows}}
}

func withNote(note, text string) string {
	if note == "" {
		return text
	}

	return note + "\n\n" + text
}

// welcomeScreen is the menu with the chat's subscription state.
func (m *Menu) welcomeScreen(chatID int64) screen {
	hour, subscribed := m.subscribers.Hour(chatID)

	return mainScreen(welcomeFor(hour, subscribed))
}

// welcomeFor is the menu text for a chat that is subscribed at hour, or is not
// subscribed.
func welcomeFor(hour int, subscribed bool) string {
	status := notSubscribedStatus
	if subscribed {
		status = fmt.Sprintf("✅ Сейчас подписка включена: каждый день в %s (МСК).", hourLabel(hour))
	}

	return welcomeText + "\n\n" + status + "\n\n" + chooseText
}

func subscribedText(hour int) string {
	return fmt.Sprintf("✅ Подписка включена. Задача дня будет приходить каждый день в %s (МСК). "+
		"Время можно поменять в настройках.", hourLabel(hour))
}

func hourLabel(hour int) string {
	return fmt.Sprintf("%02d:00", hour)
}

func backButton() models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: "⬅️ Назад", CallbackData: callbackMenu}
}

func mainScreen(text string) screen {
	return screen{
		text: text,
		markup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: "✅ Подписаться", CallbackData: callbackSubscribe},
					{Text: "🛑 Отписаться", CallbackData: callbackUnsubscribe},
				},
				{
					{Text: "⚙️ Настройки", CallbackData: callbackSettings},
					{Text: "ℹ️ О проекте", CallbackData: callbackAbout},
				},
			},
		},
	}
}

func aboutScreen() screen {
	return screen{
		text: aboutText,
		markup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "📦 Код на GitHub", URL: RepositoryURL}},
				{backButton()},
			},
		},
	}
}
