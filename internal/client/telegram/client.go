package telegram

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
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

func (c *client) SendToAll(ctx context.Context, question domain.DailyQuestion) {
	text := getPostMessageSchema(question)

	c.sendToFixed(ctx, text)

	for _, chatID := range c.subscribers.List() {
		// A subscriber who is also a fixed chat already got the post.
		if slices.Contains(c.chats, strconv.FormatInt(chatID, 10)) {
			continue
		}

		err := c.sendMessage(ctx, chatID, text)
		if errors.Is(err, bot.ErrorForbidden) {
			// Telegram says the user blocked the bot (or the chat is gone).
			c.dropSubscriber(chatID)
		}
	}
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
