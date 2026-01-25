package telegram

import (
	"context"
	"errors"
	"fmt"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/labstack/gommon/log"
)

type client struct {
	bot      *bot.Bot
	chats    []string
	taskData domain.DailyQuestion
}

func New(bot *bot.Bot, chats []string, dailyQuestion domain.DailyQuestion) (Client, error) {
	if bot == nil {
		return nil, errors.New("bot doesn't create")
	}

	if len(chats) == 0 {
		return nil, errors.New("chats array is empty")
	}

	return &client{
		chats:    chats,
		bot:      bot,
		taskData: dailyQuestion,
	}, nil
}

func (c *client) SendMessages(ctx context.Context) error {
	fmt.Println(c.chats)

	for _, chatId := range c.chats {
		c.sendMessage(ctx, chatId)
	}

	return nil
}

func (c *client) sendMessage(ctx context.Context, chatId string) {
	if c.bot == nil {
		log.Error("bot not initialized")
	}

	_, err := c.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatId,
		Text:      getPostMessageSchema(c.taskData),
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		log.Errorf("error while sending message: %v", err)
	}
}
