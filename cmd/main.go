package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/leetcode"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/telegram"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/config"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/daily"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/robfig/cron/v3"
)

func main() {
	conf, err := config.Load("config.toml")
	if err != nil {
		log.Fatalf("error while loading config: %v", err)
	}

	token, err := config.LoadToken(".env", os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	store, err := subscribers.Open(conf.Storage.SubscribersPath)
	if err != nil {
		log.Fatalf("error while opening subscribers store: %v", err)
	}

	moscowLoc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Fatal("Ошибка загрузки таймзоны:", err)
	}

	tgBot, err := bot.New(token)
	if err != nil {
		log.Fatalf("error while creating bot: %v", err)
	}

	menu := telegram.NewMenu(tgBot, store)
	menu.Register(tgBot)

	tgClient, err := telegram.New(tgBot, conf.Telegram.TelegramChats, store)
	if err != nil {
		log.Fatalf("error while creating telegram client: %v", err)
	}

	leetcodeClient := leetcode.New("https://leetcode.com")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := menu.Setup(ctx); err != nil {
		// The bot still works without its profile text, so keep going.
		log.Printf("error while setting up the bot profile: %v", err)
	}

	c := cron.New(cron.WithLocation(moscowLoc))
	schedule := daily.NewSchedule(leetcodeClient, tgClient, time.Now().In(moscowLoc))

	// Every full hour is a delivery window: subscribers get the post at the
	// hour they chose, the fixed chats at 11:00. The first run after the host
	// wakes up also serves the windows it slept through today.
	_, err = c.AddFunc("0 * * * *", func() {
		now := time.Now().In(moscowLoc)
		log.Printf("Запуск ежедневной задачи в %s...", now.Format("15:04"))
		schedule.Run(ctx, now)
	})
	if err != nil {
		log.Fatal("Ошибка настройки cron:", err)
	}

	// The start-up post goes to the fixed chats only, so a restart does not
	// send subscribers a second copy of the same problem.
	daily.Post(ctx, leetcodeClient, tgClient.SendToFixedChats)

	c.Start()
	log.Println("Планировщик запущен. Ожидание задач...")

	tgBot.Start(ctx)

	<-c.Stop().Done()
	log.Println("Планировщик остановлен")
}
