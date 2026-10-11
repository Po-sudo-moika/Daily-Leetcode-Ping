package telegram

import (
	"context"
	"errors"
	"strconv"

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

func (c *client) HasRecipients(hour int) bool {
	fixed, subscribed := c.recipients(hour)

	return len(fixed) > 0 || len(subscribed) > 0
}

func (c *client) SendForHour(ctx context.Context, hour int, question domain.DailyQuestion) {
	text := getPostMessageSchema(question)
	fixed, subscribed := c.recipients(hour)

	for _, chat := range fixed {
		c.sendMessage(ctx, chat, text)
	}

	for _, chatID := range subscribed {
		err := c.sendMessage(ctx, chatID, text)
		if errors.Is(err, bot.ErrorForbidden) {
			// Telegram says the user blocked the bot (or the chat is gone).
			c.dropSubscriber(chatID)
		}
	}
}

// recipients returns who gets the post at this Moscow hour: the fixed chats
// at fixedChatsHour, and the subscribers who chose this hour.
func (c *client) recipients(hour int) (fixed []string, subscribed []int64) {
	if hour == fixedChatsHour {
		fixed = c.fixedOnly()
	}

	return fixed, c.subscribers.ListAtHour(hour)
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
