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
	// SendToAll posts to the fixed chats and to every subscriber.
	SendToAll(ctx context.Context, question domain.DailyQuestion)
}

// API is the part of *bot.Bot that this package uses. Tests replace it.
type API interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *bot.AnswerCallbackQueryParams) (bool, error)
}

// Subscribers is the subscription storage.
type Subscribers interface {
	Add(chatID int64) (bool, error)
	Remove(chatID int64) (bool, error)
	List() []int64
}
