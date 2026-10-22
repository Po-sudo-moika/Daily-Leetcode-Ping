package telegram

import (
	"context"
	"errors"
	"strconv"
	"strings"
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
	}, nil
}

func (c *client) SendToFixedChats(ctx context.Context, now time.Time, question domain.DailyQuestion) error {
	text := getPostMessageSchema(question)
	day := now.Format(time.DateOnly)

	var errs []error
	for _, chat := range c.chats {
		err := c.sendMessage(ctx, chat, text)
		if hour, _ := c.deliveryHour(chat); err == nil && now.Hour() >= hour {
			errs = append(errs, c.subscribers.MarkDelivered(chat, day))
		}
	}

	return errors.Join(errs...)
}

// fixedChatsHour is the Moscow hour at which the chats from config.toml get
// the daily post.
const fixedChatsHour = subscribers.DefaultHour

func (c *client) HasDue(now time.Time) bool {
	fixed, subscribed := c.due(now)

	return len(fixed) > 0 || len(subscribed) > 0
}

// SendDue records a delivery only after Telegram accepted the message, so a
// chat that failed is due again at the next run.
func (c *client) SendDue(ctx context.Context, now time.Time, question domain.DailyQuestion) error {
	text := getPostMessageSchema(question)
	day := now.Format(time.DateOnly)
	fixed, subscribed := c.due(now)

	var errs []error
	for _, chat := range fixed {
		if err := c.sendMessage(ctx, chat, text); err == nil {
			errs = append(errs, c.subscribers.MarkDelivered(chat, day))
		}
	}

	for _, chatID := range subscribed {
		err := c.sendMessage(ctx, chatID, text)
		switch {
		case err == nil:
			errs = append(errs, c.subscribers.MarkDelivered(strconv.FormatInt(chatID, 10), day))
		case chatUnreachable(err):
			c.dropSubscriber(chatID)
		}
	}

	return errors.Join(errs...)
}

// due returns who is due for the post at now: the fixed chats and the
// subscribers whose hour has come, each only if they have not got the post
// today.
func (c *client) due(now time.Time) (fixed []string, subscribed []int64) {
	day := now.Format(time.DateOnly)

	for _, chat := range c.fixedOnly() {
		if hour, _ := c.deliveryHour(chat); now.Hour() >= hour && !c.subscribers.Delivered(chat, day) {
			fixed = append(fixed, chat)
		}
	}

	return fixed, c.subscribers.Due(now.Hour(), day)
}

// fixedOnly returns the fixed chats that have no subscription of their own. A
// fixed chat that is also a subscriber gets the post once a day, at the hour
// it chose, instead of at the fixed hour as well.
func (c *client) fixedOnly() []string {
	var chats []string
	for _, chat := range c.chats {
		if _, subscribed := c.deliveryHour(chat); !subscribed {
			chats = append(chats, chat)
		}
	}

	return chats
}

// deliveryHour returns the Moscow delivery hour of a fixed chat: the hour it
// chose when it is also a subscriber, fixedChatsHour otherwise. The second
// result reports whether it is a subscriber.
func (c *client) deliveryHour(chat string) (int, bool) {
	if chatID, err := strconv.ParseInt(chat, 10, 64); err == nil {
		if hour, subscribed := c.subscribers.Hour(chatID); subscribed {
			return hour, true
		}
	}

	return fixedChatsHour, false
}

// chatUnreachable reports an error from Telegram that says the chat will not
// get a post any more: the user blocked the bot, the bot was kicked from the
// group or channel, or the chat does not exist. Any other error, such as a
// timeout, may pass, so it only fails this round.
func chatUnreachable(err error) bool {
	if errors.Is(err, bot.ErrorForbidden) {
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
