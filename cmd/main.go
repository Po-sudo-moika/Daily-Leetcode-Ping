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

	ownerID, err := config.LoadOwnerID(".env", os.Getenv, conf.Telegram.OwnerID)
	if err != nil {
		log.Fatal(err)
	}
	if ownerID == 0 {
		log.Println("Владелец бота не задан (telegram.ownerId или TELEGRAM_OWNER_ID): команда /chats отключена")
	}

	store, err := subscribers.Open(conf.Storage.SubscribersPath)
	if err != nil {
		log.Fatalf("error while opening subscribers store: %v", err)
	}

	moscowLoc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Fatal("Ошибка загрузки таймзоны:", err)
	}

	tgBot, err := bot.New(token, telegram.BotOptions(store)...)
	if err != nil {
		log.Fatalf("error while creating bot: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fixedChats := telegram.ResolveFixedChats(ctx, tgBot, conf.Telegram.TelegramChats)

	menu := telegram.NewMenu(tgBot, store, fixedChats).WithOwner(ownerID)
	menu.Register(tgBot)

	tgClient, err := telegram.New(tgBot, fixedChats, store)
	if err != nil {
		log.Fatalf("error while creating telegram client: %v", err)
	}

	// A failed fetch is not repeated for an hour, though the round runs every minute.
	leetcodeClient := leetcode.NewBackoff(leetcode.New("https://leetcode.com"), time.Hour)

	if err := menu.Setup(ctx); err != nil {
		// The bot still works without its profile text, so keep going.
		log.Printf("error while setting up the bot profile: %v", err)
	}

	// A round still running when the next one starts makes the next one skip,
	// so two rounds never send the same post; the round after catches up.
	c := cron.New(cron.WithLocation(moscowLoc), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))

	// Every minute, everyone whose delivery time has come and who has not got
	// today's post gets it: subscribers from the time they chose, the fixed
	// chats from 11:00. So a post is never more than a minute late, and the
	// first round after the host wakes up, or after a restart, catches up on
	// the times it missed. A round with nobody due does nothing.
	_, err = c.AddFunc("* * * * *", func() {
		daily.Deliver(ctx, leetcodeClient, tgClient, time.Now().In(moscowLoc))
	})
	if err != nil {
		log.Fatal("Ошибка настройки cron:", err)
	}

	// The start-up post goes to the fixed chats only, so a restart does not
	// send subscribers a second copy of the same problem. Once a chat's
	// delivery hour has come it counts as its post for the day.
	daily.Post(ctx, leetcodeClient, func(ctx context.Context, question domain.DailyQuestion) error {
		return tgClient.SendToFixedChats(ctx, time.Now().In(moscowLoc), question)
	})

	c.Start()
	log.Println("Планировщик запущен. Ожидание задач...")

	tgBot.Start(ctx)

	<-c.Stop().Done()
	log.Println("Планировщик остановлен")
}
