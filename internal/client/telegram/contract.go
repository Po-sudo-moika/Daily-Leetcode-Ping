package telegram

import (
	"context"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Client posts the daily question.
type Client interface {
	// SendToFixedChats posts only to the chats from config.toml. Once a
	// chat's delivery time has come at now, a Moscow time, the post counts as
	// its delivery for the day, so the next round does not repeat it. That
	// time is the one it chose when it is also a subscriber, 11:00 otherwise.
	SendToFixedChats(ctx context.Context, now time.Time, question domain.DailyQuestion) error
	// HasDue reports whether anyone is due for today's post at now, a Moscow
	// time: their delivery time has come, they have not got the post today and
	// no send to them failed earlier in the same clock hour.
	HasDue(now time.Time) bool
	// SendDue posts to everyone due at now, a Moscow time: the subscribers
	// whose chosen time has come, and the chats from config.toml once their
	// fixed time has come. It records each successful delivery.
	SendDue(ctx context.Context, now time.Time, question domain.DailyQuestion) error
}

// API is the part of *bot.Bot that this package uses. Tests replace it.
type API interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	EditMessageText(ctx context.Context, params *bot.EditMessageTextParams) (*models.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *bot.AnswerCallbackQueryParams) (bool, error)
	SetMyCommands(ctx context.Context, params *bot.SetMyCommandsParams) (bool, error)
	SetMyDescription(ctx context.Context, params *bot.SetMyDescriptionParams) (bool, error)
	SetMyShortDescription(ctx context.Context, params *bot.SetMyShortDescriptionParams) (bool, error)
	GetChat(ctx context.Context, params *bot.GetChatParams) (*models.ChatFullInfo, error)
	GetChatMember(ctx context.Context, params *bot.GetChatMemberParams) (*models.ChatMember, error)
}

// Subscribers is the subscription storage.
type Subscribers interface {
	// Add subscribes a chat because someone asked for it. It also takes the
	// chat out of the ones that unsubscribed.
	Add(chatID int64) (bool, error)
	// Meet subscribes a chat seen for the first time. A subscriber and a
	// chat that unsubscribed are left as they are.
	Meet(chatID int64) (bool, error)
	// Remove unsubscribes a chat because someone chose to; Meet does not
	// subscribe it again.
	Remove(chatID int64) (bool, error)
	// Drop unsubscribes a chat that cannot get the post any more. It is not
	// a choice: Meet subscribes the chat again when it comes back.
	Drop(chatID int64) (bool, error)
	// Migrate moves a group's subscription and time, or its Unsubscribe, to
	// the new id Telegram gave it as a supergroup.
	Migrate(from, to int64) (bool, error)
	// Time returns the Moscow delivery time of a chat, in minutes since
	// midnight (see subscribers.At); false when not subscribed.
	Time(chatID int64) (int, bool)
	SetTime(chatID int64, t int) error
	// Due returns the subscribers whose time is at or before now, in minutes
	// since midnight, and who have not got the post on day.
	Due(now int, day string) []int64
	// Known returns the subscribed chats and the ones that unsubscribed.
	Known() []int64
	// Language returns the language of a chat: Russian until it switches.
	Language(chatID int64) subscribers.Language
	SetLanguage(chatID int64, lang subscribers.Language) error
	// Greeted reports whether the chat was sent the hello of its first /start.
	Greeted(chatID int64) bool
	MarkGreeted(chatID int64) error
	// Delivered reports whether chat got the post on day.
	Delivered(chat, day string) bool
	// LastDelivered returns the last day chat got the post; "" for none.
	LastDelivered(chat string) string
	// MarkDelivered records that chat got the post on day.
	MarkDelivered(chat, day string) error
}
