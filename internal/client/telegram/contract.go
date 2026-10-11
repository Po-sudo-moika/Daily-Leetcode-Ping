package telegram

import (
	"context"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Client posts the daily question.
type Client interface {
	// SendToFixedChats posts only to the chats from config.toml.
	SendToFixedChats(ctx context.Context, question domain.DailyQuestion)
	// HasRecipients reports whether anyone gets the post at this Moscow hour.
	HasRecipients(hour int) bool
	// SendForHour posts to everyone whose delivery hour is this Moscow hour:
	// the subscribers who chose it, and the chats from config.toml at their
	// fixed hour.
	SendForHour(ctx context.Context, hour int, question domain.DailyQuestion)
}

// API is the part of *bot.Bot that this package uses. Tests replace it.
type API interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	EditMessageText(ctx context.Context, params *bot.EditMessageTextParams) (*models.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *bot.AnswerCallbackQueryParams) (bool, error)
	SetMyCommands(ctx context.Context, params *bot.SetMyCommandsParams) (bool, error)
	SetMyDescription(ctx context.Context, params *bot.SetMyDescriptionParams) (bool, error)
	SetMyShortDescription(ctx context.Context, params *bot.SetMyShortDescriptionParams) (bool, error)
}

// Subscribers is the subscription storage.
type Subscribers interface {
	Add(chatID int64) (bool, error)
	Remove(chatID int64) (bool, error)
	// Hour returns the Moscow delivery hour; false when not subscribed.
	Hour(chatID int64) (int, bool)
	SetHour(chatID int64, hour int) error
	ListAtHour(hour int) []int64
}
