package telegram

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/labstack/gommon/log"
)

// The limits against abuse; see docs/security.md. A burst is how many actions
// pass at once, the rate is how fast the allowance comes back.
const (
	userBurst     = 10
	userPerMinute = 20
	// chatBurst and chatPerMinute limit a group as a whole, so many users in one
	// group cannot flood it. Telegram allows a group 20 messages a minute.
	chatBurst     = 10
	chatPerMinute = 15
	// lookupBurst and lookupPerMinute limit the commands that make the bot ask
	// LeetCode, per user.
	lookupBurst     = 5
	lookupPerMinute = 5

	// noticeWindow is how seldom a person or group gets the "too many requests"
	// notice: at most once in it.
	noticeWindow = time.Minute

	// maxCommandRunes is the longest command the bot reads. A longer one is
	// ignored.
	maxCommandRunes = 256

	// handlerWorkers is how many updates the bot handles at the same time.
	handlerWorkers = 8
)

var (
	tooManyText = message{
		ru: "Слишком много запросов. Попробуй ещё раз через минуту.",
		en: "Too many requests. Please try again in a minute.",
	}

	tooManyLookupsText = message{
		ru: "Слишком много проверок LeetCode. Попробуй ещё раз через минуту.",
		en: "Too many LeetCode lookups. Please try again in a minute.",
	}
)

// limiter is a token bucket for each key. It is safe for concurrent use.
type limiter struct {
	burst     float64
	perSecond float64

	mu      sync.Mutex
	buckets map[int64]*bucket
	swept   time.Time
}

type bucket struct {
	tokens  float64
	seen    time.Time
	noticed time.Time
}

func newLimiter(burst int, perMinute float64) *limiter {
	return &limiter{burst: float64(burst), perSecond: perMinute / 60, buckets: make(map[int64]*bucket)}
}

// allow takes one token from the bucket of key. Without a token it reports
// false, and notify is true at most once in noticeWindow, so the caller tells
// the person once.
func (l *limiter) allow(key int64, now time.Time) (ok, notify bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sweep(now)

	b, found := l.buckets[key]
	if !found {
		b = &bucket{tokens: l.burst, seen: now}
		l.buckets[key] = b
	}

	b.tokens = min(l.burst, b.tokens+max(0, now.Sub(b.seen).Seconds())*l.perSecond)
	b.seen = now

	if b.tokens >= 1 {
		b.tokens--
		return true, false
	}

	if b.noticed.IsZero() || now.Sub(b.noticed) >= noticeWindow {
		b.noticed = now
		return false, true
	}

	return false, false
}

// sweep forgets the keys idle for two windows, once a minute. By then a bucket
// is full again, so forgetting it changes nothing, and the map stays small. The
// caller holds l.mu.
func (l *limiter) sweep(now time.Time) {
	if now.Sub(l.swept) < noticeWindow {
		return
	}

	l.swept = now
	for key, b := range l.buckets {
		if now.Sub(b.seen) >= 2*noticeWindow {
			delete(l.buckets, key)
		}
	}
}

// Limits holds the rate limits of the bot: how many commands and button presses
// a user, and a group, may send, and how many LeetCode lookups a user may ask.
// The numbers are constants of this package. Create it with NewLimits, give it
// to BotOptions and to Menu.WithLimits.
type Limits struct {
	users   *limiter
	chats   *limiter
	lookups *limiter
	now     func() time.Time

	// api sends the notices, and forBot tells whether a command is addressed to
	// this bot; Menu.WithLimits sets both before the bot starts. With no forBot,
	// every command counts.
	api    API
	forBot func(*models.Message) bool
}

// NewLimits returns the limits with the default numbers.
func NewLimits() *Limits {
	return &Limits{
		users:   newLimiter(userBurst, userPerMinute),
		chats:   newLimiter(chatBurst, chatPerMinute),
		lookups: newLimiter(lookupBurst, lookupPerMinute),
		now:     time.Now,
	}
}

// allowLookup reports whether the user may ask LeetCode now. Without a lookup,
// notify is true at most once in noticeWindow, so the caller tells the user
// once. Nil limits allow everything.
func (l *Limits) allowLookup(userID int64) (ok, notify bool) {
	if l == nil {
		return true, false
	}

	return l.lookups.allow(userID, l.now())
}

// guardUpdates is the first middleware. It drops a command that is too long,
// and, with limits, a command or button press over the limit of its user or its
// group. A button press that is dropped is still answered, so Telegram shows no
// spinner. The person gets a short notice at most once in noticeWindow: for a
// button it is the text of the answer, for a command a message in the same
// chat. Other updates, such as the bot being added to a chat, pass: they cost
// no answer, and dropping a "removed" update would leave a dead chat on the
// list. limits may be nil.
func guardUpdates(limits *Limits, subscribers Subscribers) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if isTooLong(update) {
				return
			}

			if limits != nil && !limits.admit(ctx, update, subscribers) {
				return
			}

			next(ctx, b, update)
		}
	}
}

// isTooLong reports a command longer than maxCommandRunes.
func isTooLong(update *models.Update) bool {
	msg := update.Message

	return msg != nil && strings.HasPrefix(msg.Text, "/") && utf8.RuneCountInString(msg.Text) > maxCommandRunes
}

// admit counts the update against the limits and reports whether it may go on.
func (l *Limits) admit(ctx context.Context, update *models.Update, subscribers Subscribers) bool {
	userID, chatID, ok := l.actionOf(update)
	if !ok {
		return true
	}

	now := l.now()
	// A user id of 0 is an update without a sender, such as a channel post.
	if userID != 0 {
		if allowed, notify := l.users.allow(userID, now); !allowed {
			l.reject(ctx, update, chatID, notify, subscribers)
			return false
		}
	}

	// Groups, supergroups and channels have negative ids.
	if chatID < 0 {
		if allowed, notify := l.chats.allow(chatID, now); !allowed {
			l.reject(ctx, update, chatID, notify, subscribers)
			return false
		}
	}

	return true
}

// actionOf returns the user and the chat of a command to this bot or of a button
// press. ok is false for any other update, such as a command to another bot.
func (l *Limits) actionOf(update *models.Update) (userID, chatID int64, ok bool) {
	switch {
	case update.Message != nil && strings.HasPrefix(update.Message.Text, "/"):
		if l.forBot != nil && !l.forBot(update.Message) {
			return 0, 0, false
		}

		if from := update.Message.From; from != nil {
			userID = from.ID
		}

		return userID, update.Message.Chat.ID, true
	case update.CallbackQuery != nil:
		// The chat is 0 for a button whose message Telegram no longer gives.
		chat, _ := updateChat(update, 0)

		return update.CallbackQuery.From.ID, chat.ID, true
	}

	return 0, 0, false
}

// reject drops the update: a button press is answered, with the notice as the
// answer text when notify is set, and a command gets the notice as a message
// when notify is set.
func (l *Limits) reject(ctx context.Context, update *models.Update, chatID int64, notify bool, subscribers Subscribers) {
	if l.api == nil {
		return
	}

	text := ""
	if notify {
		text = tooManyText.in(subscribers.Language(chatID))
	}

	if query := update.CallbackQuery; query != nil {
		if _, err := l.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: text}); err != nil {
			log.Errorf("error while answering a dropped callback query: %v", err)
		}

		return
	}

	if notify {
		if _, err := l.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text}); err != nil {
			log.Errorf("error while sending the rate limit notice to %d: %v", chatID, err)
		}
	}
}
