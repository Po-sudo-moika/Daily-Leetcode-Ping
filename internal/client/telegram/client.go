package telegram

import (
	"context"
	"errors"
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

func (c *client) SendToFixedChats(ctx context.Context, question domain.DailyQuestion) error {
	text := getPostMessageSchema(question)

	for _, chatID := range c.chats {
		c.sendMessage(ctx, chatID, text)
	}

	return nil
}

func (c *client) SendToAll(ctx context.Context, question domain.DailyQuestion) error {
	text := getPostMessageSchema(question)

	sent := make(map[string]struct{}, len(c.chats))
	for _, chatID := range c.chats {
		sent[chatID] = struct{}{}
		c.sendMessage(ctx, chatID, text)
	}

	for _, chatID := range c.subscribers.List() {
		key := strconv.FormatInt(chatID, 10)
		if _, ok := sent[key]; ok {
			continue
		}
		sent[key] = struct{}{}

		err := c.sendMessage(ctx, chatID, text)
		if errors.Is(err, bot.ErrorForbidden) {
			// Telegram says the user blocked the bot (or the chat is gone).
			c.dropSubscriber(chatID)
		}
	}

	return nil
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
