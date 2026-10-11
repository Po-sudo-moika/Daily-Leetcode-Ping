package telegram

import (
	"context"
	"errors"
	"strconv"
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

func (c *client) SendToFixedChats(ctx context.Context, question domain.DailyQuestion) {
	c.sendToFixed(ctx, getPostMessageSchema(question))
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
		case errors.Is(err, bot.ErrorForbidden):
			// Telegram says the user blocked the bot (or the chat is gone).
			c.dropSubscriber(chatID)
		}
	}

	return errors.Join(errs...)
}

// due returns who is due for the post at now: the fixed chats once
// fixedChatsHour has come, and the subscribers whose hour has come, each only
// if they have not got the post today.
func (c *client) due(now time.Time) (fixed []string, subscribed []int64) {
	day := now.Format(time.DateOnly)

	if now.Hour() >= fixedChatsHour {
		for _, chat := range c.fixedOnly() {
			if !c.subscribers.Delivered(chat, day) {
				fixed = append(fixed, chat)
			}
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
		if chatID, err := strconv.ParseInt(chat, 10, 64); err == nil {
			if _, subscribed := c.subscribers.Hour(chatID); subscribed {
				continue
			}
		}
		chats = append(chats, chat)
	}

	return chats
}

func (c *client) sendToFixed(ctx context.Context, text string) {
	for _, chatID := range c.chats {
		c.sendMessage(ctx, chatID, text)
	}
}

func (c *client) dropSubscriber(chatID int64) {
	if _, err := c.subscribers.Remove(chatID); err != nil {
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
