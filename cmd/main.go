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
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
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

	// A round still running when the next one starts makes the next one skip,
	// so two rounds never send the same post; the round after catches up.
	c := cron.New(cron.WithLocation(moscowLoc), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))

	// Every full hour, everyone whose delivery hour has come and who has not
	// got today's post gets it: subscribers from the hour they chose, the
	// fixed chats from 11:00. So the first round after the host wakes up, or
	// after a restart, catches up on the hours it missed.
	_, err = c.AddFunc("0 * * * *", func() {
		now := time.Now().In(moscowLoc)
		log.Printf("Запуск ежедневной задачи в %s...", now.Format("15:04"))
		daily.Deliver(ctx, leetcodeClient, tgClient, now)
	})
	if err != nil {
		log.Fatal("Ошибка настройки cron:", err)
	}

	// The start-up post goes to the fixed chats only, so a restart does not
	// send subscribers a second copy of the same problem.
	daily.Post(ctx, leetcodeClient, func(ctx context.Context, question domain.DailyQuestion) error {
		tgClient.SendToFixedChats(ctx, question)
		return nil
	})

	c.Start()
	log.Println("Планировщик запущен. Ожидание задач...")

	tgBot.Start(ctx)

	<-c.Stop().Done()
	log.Println("Планировщик остановлен")
}
