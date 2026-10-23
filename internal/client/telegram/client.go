package telegram

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/labstack/gommon/log"
)

type client struct {
	api         API
	chats       []string
	subscribers Subscribers

	mu sync.Mutex
	// failedAt maps a chat to the time of its last failed send. It is kept in
	// memory only, so a restart tries every due chat again.
	failedAt map[string]time.Time
}

func New(api API, chats []string, subscribers Subscribers) (Client, error) {
	if api == nil {
		return nil, errors.New("bot doesn't create")
	}

	if subscribers == nil {
		return nil, errors.New("subscribers storage is nil")
	}

	return &client{
		api:         api,
		chats:       chats,
		subscribers: subscribers,
		failedAt:    make(map[string]time.Time),
	}, nil
}

// ResolveFixedChats looks up the id of each fixed chat given by its @name,
// with one getChat call each. From then on the chat is matched by its id, like
// a fixed chat given by id: if it is also a subscriber, it gets one post a day,
// at the time it chose. A name that cannot be looked up is logged and kept, so
// the chat still gets the post by name.
func ResolveFixedChats(ctx context.Context, api API, chats []string) []string {
	resolved := make([]string, 0, len(chats))
	for _, chat := range chats {
		if strings.HasPrefix(chat, "@") {
			info, err := api.GetChat(ctx, &bot.GetChatParams{ChatID: chat})
			if err != nil {
				log.Errorf("fixed chat %s could not be looked up, it gets the post by name: %v", chat, err)
			} else {
				chat = strconv.FormatInt(info.ID, 10)
			}
		}
		resolved = append(resolved, chat)
	}

	return resolved
}

func (c *client) SendToFixedChats(ctx context.Context, now time.Time, question domain.DailyQuestion) error {
	day := now.Format(time.DateOnly)

	var errs []error
	for _, chat := range c.chats {
		err := c.sendMessage(ctx, chat, c.post(chat, question))
		at, _ := c.deliveryTime(chat)
		timeHasCome := subscribers.Now(now) >= at
		if err == nil || timeHasCome {
			c.noteSend(chat, now, err)
		}
		if err == nil && timeHasCome {
			errs = append(errs, c.subscribers.MarkDelivered(chat, day))
		}
	}

	return errors.Join(errs...)
}

// post is the daily post for a chat, in the chat's language. A fixed chat given
// by its @name has no id to look up, so it gets Russian.
func (c *client) post(chat string, question domain.DailyQuestion) string {
	lang := subscribers.Russian
	if chatID, err := strconv.ParseInt(chat, 10, 64); err == nil {
		lang = c.subscribers.Language(chatID)
	}

	return getPostMessageSchema(question, lang)
}

// fixedChatsTime is the Moscow time at which the chats from config.toml get
// the daily post.
const fixedChatsTime = subscribers.DefaultTime

func (c *client) HasDue(now time.Time) bool {
	fixed, subscribed := c.due(now)

	return len(fixed) > 0 || len(subscribed) > 0
}

// SendDue records a delivery only after Telegram accepted the message, so a
// chat that failed is due again at the next run.
func (c *client) SendDue(ctx context.Context, now time.Time, question domain.DailyQuestion) error {
	day := now.Format(time.DateOnly)
	fixed, subscribed := c.due(now)

	var errs []error
	for _, chat := range fixed {
		err := c.sendMessage(ctx, chat, c.post(chat, question))
		c.noteSend(chat, now, err)
		if err == nil {
			errs = append(errs, c.subscribers.MarkDelivered(chat, day))
		}
	}

	for _, chatID := range subscribed {
		chat := strconv.FormatInt(chatID, 10)
		err := c.sendMessage(ctx, chatID, c.post(chat, question))
		c.noteSend(chat, now, err)
		switch {
		case err == nil:
			errs = append(errs, c.subscribers.MarkDelivered(chat, day))
		case chatUnreachable(err):
			c.dropSubscriber(chatID)
		}
	}

	return errors.Join(errs...)
}

// due returns who is due for the post at now: the fixed chats and the
// subscribers whose time has come, each only if they have not got the post
// today and no send to them failed earlier in the same clock hour. The round
// runs every minute, so without that pause a chat that Telegram refuses would
// be tried every minute of the day; a failed send is tried again at the first
// round of the next hour.
func (c *client) due(now time.Time) (fixed []string, subscribed []int64) {
	day := now.Format(time.DateOnly)
	at := subscribers.Now(now)

	for _, chat := range c.fixedOnly() {
		if t, _ := c.deliveryTime(chat); at >= t && !c.subscribers.Delivered(chat, day) && !c.failedRecently(chat, now) {
			fixed = append(fixed, chat)
		}
	}

	for _, chatID := range c.subscribers.Due(at, day) {
		if !c.failedRecently(strconv.FormatInt(chatID, 10), now) {
			subscribed = append(subscribed, chatID)
		}
	}

	return fixed, subscribed
}

// noteSend remembers a failed send to chat at now, and forgets an earlier
// failure once a send works.
func (c *client) noteSend(chat string, now time.Time, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err == nil {
		delete(c.failedAt, chat)
		return
	}

	c.failedAt[chat] = now
}

func (c *client) failedRecently(chat string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	failed, ok := c.failedAt[chat]

	return ok && clockHour(failed) == clockHour(now)
}

// clockHour names the clock hour of t, like "2026-10-11 09".
func clockHour(t time.Time) string {
	return t.Format("2006-01-02 15")
}

// fixedOnly returns the fixed chats that have no subscription of their own. A
// fixed chat that is also a subscriber gets the post once a day, at the time
// it chose, instead of at the fixed time as well.
func (c *client) fixedOnly() []string {
	var chats []string
	for _, chat := range c.chats {
		if _, subscribed := c.deliveryTime(chat); !subscribed {
			chats = append(chats, chat)
		}
	}

	return chats
}

// deliveryTime returns the Moscow delivery time of a fixed chat: the time it
// chose when it is also a subscriber, fixedChatsTime otherwise. The second
// result reports whether it is a subscriber.
func (c *client) deliveryTime(chat string) (int, bool) {
	if chatID, err := strconv.ParseInt(chat, 10, 64); err == nil {
		if t, subscribed := c.subscribers.Time(chatID); subscribed {
			return t, true
		}
	}

	return fixedChatsTime, false
}

// chatUnreachable reports an error from Telegram that says the chat will not
// get a post any more: the user blocked the bot, the bot was kicked from the
// group or channel, the chat does not exist, or the group moved to a new id
// when it became a supergroup. Any other error, such as a timeout, may pass,
// so it only fails this round.
func chatUnreachable(err error) bool {
	var migrated *bot.MigrateError
	if errors.Is(err, bot.ErrorForbidden) || errors.As(err, &migrated) {
		return true
	}

	notFound := errors.Is(err, bot.ErrorBadRequest) || errors.Is(err, bot.ErrorNotFound)

	return notFound && strings.Contains(strings.ToLower(err.Error()), "chat not found")
}

func (c *client) dropSubscriber(chatID int64) {
	if _, err := c.subscribers.Drop(chatID); err != nil {
		log.Errorf("error while dropping subscriber %d: %v", chatID, err)
		return
	}

	log.Infof("subscriber %d dropped: Telegram refused delivery", chatID)
}

func (c *client) sendMessage(ctx context.Context, chatID any, text string) error {
	_, err := c.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		log.Errorf("error while sending message to %v: %v", chatID, err)
	}

	return err
}
