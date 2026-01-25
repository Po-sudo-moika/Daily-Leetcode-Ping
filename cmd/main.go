package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/leetcode"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/telegram"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/config"
	"github.com/go-telegram/bot"
	"gopkg.in/robfig/cron.v2"
)

func main() {
	c := cron.New()

	_, err := c.AddFunc("0 10 * * *", func() {
		log.Println("Запуск ежедневной задачи...")
		sendDailyPost()
	})

	if err != nil {
		log.Fatal("Ошибка настройки cron:", err)
	}

	sendDailyPost()

	c.Start()
	log.Println("Планировщик запущен. Ожидание задач...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	c.Stop()
	log.Println("Планировщик остановлен")
}

func sendDailyPost() {
	var conf config.Config
	configPath := "config.toml"

	if _, err := toml.DecodeFile(configPath, &conf); err != nil {
		log.Fatalf("error while decoding config file: %v", err)
	}

	tgBot, err := bot.New(conf.Telegram.BotCode)
	if err != nil {
		log.Fatal("error while creating bot")
		return
	}

	leetcodeClient := leetcode.New("https://leetcode.com")

	ctx := context.Background()

	res, err := leetcodeClient.GetCurrentDailyTask(ctx)
	if err != nil {
		log.Fatal(err)
		return
	}

	tgClient, err := telegram.New(tgBot, conf.Telegram.TelegramChats, res.Result.ActiveDailyCodingChallengeQuestion)

	tgClient.SendMessages(ctx)
}
