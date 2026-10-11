package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/labstack/gommon/log"
)

const (
	callbackSubscribe   = "subscribe"
	callbackUnsubscribe = "unsubscribe"
	callbackAbout       = "about"

	// RepositoryURL is the public code of the project.
	RepositoryURL = "https://github.com/Po-sudo-moika/Daily-Leetcode-Ping"

	welcomeText = "👋 Привет! Я присылаю задачу дня с LeetCode каждый день в 11:00 по Москве.\n\nВыбери действие:"

	subscribedText = "✅ Подписка включена. Задача дня будет приходить каждый день в 11:00 (МСК)."

	unsubscribedText = "🛑 Подписка отключена. Чтобы вернуться, нажми «Подписаться»."

	saveFailedText = "⚠️ Не получилось сохранить выбор. Попробуй ещё раз чуть позже."

	aboutText = "ℹ️ Daily LeetCode Ping — небольшой бот, который каждый день в 11:00 (МСК) " +
		"присылает задачу дня с leetcode.com: название, сложность и ссылку.\n\n" +
		"Код открыт — смотри на GitHub."
)

// Menu answers /start and the buttons of the inline menu.
type Menu struct {
	api         API
	subscribers Subscribers
}

func NewMenu(api API, subscribers Subscribers) *Menu {
	return &Menu{api: api, subscribers: subscribers}
}

// Register wires the menu handlers into the bot.
func (m *Menu) Register(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeMessageText, "start", bot.MatchTypeCommandStartOnly, m.HandleStart)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackSubscribe, bot.MatchTypeExact, m.HandleSubscribe)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackUnsubscribe, bot.MatchTypeExact, m.HandleUnsubscribe)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackAbout, bot.MatchTypeExact, m.HandleAbout)
}

func (m *Menu) HandleStart(ctx context.Context, _ *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	m.send(ctx, update.Message.Chat.ID, welcomeText, menuKeyboard())
}

func (m *Menu) HandleSubscribe(ctx context.Context, _ *bot.Bot, update *models.Update) {
	chatID, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	if _, err := m.subscribers.Add(chatID); err != nil {
		log.Errorf("error while subscribing %d: %v", chatID, err)
		m.send(ctx, chatID, saveFailedText, menuKeyboard())
		return
	}

	m.send(ctx, chatID, subscribedText, menuKeyboard())
}

func (m *Menu) HandleUnsubscribe(ctx context.Context, _ *bot.Bot, update *models.Update) {
	chatID, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	if _, err := m.subscribers.Remove(chatID); err != nil {
		log.Errorf("error while unsubscribing %d: %v", chatID, err)
		m.send(ctx, chatID, saveFailedText, menuKeyboard())
		return
	}

	m.send(ctx, chatID, unsubscribedText, menuKeyboard())
}

func (m *Menu) HandleAbout(ctx context.Context, _ *bot.Bot, update *models.Update) {
	chatID, ok := m.ackCallback(ctx, update)
	if !ok {
		return
	}

	m.send(ctx, chatID, aboutText, aboutKeyboard())
}

// ackCallback stops the button spinner and returns the chat of the pressed
// button. It reports false when the update has no usable chat.
func (m *Menu) ackCallback(ctx context.Context, update *models.Update) (int64, bool) {
	query := update.CallbackQuery
	if query == nil {
		return 0, false
	}

	if _, err := m.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
		log.Errorf("error while answering callback query: %v", err)
	}

	// Telegram sends an inaccessible message when the menu message is too old.
	// It still carries the chat, so the buttons keep working.
	switch {
	case query.Message.Message != nil:
		return query.Message.Message.Chat.ID, true
	case query.Message.InaccessibleMessage != nil:
		return query.Message.InaccessibleMessage.Chat.ID, true
	}

	return 0, false
}

func (m *Menu) send(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	_, err := m.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: markup,
	})
	if err != nil {
		log.Errorf("error while sending menu message to %d: %v", chatID, err)
	}
}

func menuKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "✅ Подписаться", CallbackData: callbackSubscribe},
				{Text: "🛑 Отписаться", CallbackData: callbackUnsubscribe},
			},
			{
				{Text: "ℹ️ О проекте", CallbackData: callbackAbout},
			},
		},
	}
}

func aboutKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📦 Код на GitHub", URL: RepositoryURL},
			},
		},
	}
}
