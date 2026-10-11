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

	telegram.NewMenu(tgBot, store).Register(tgBot)

	tgClient, err := telegram.New(tgBot, conf.Telegram.TelegramChats, store)
	if err != nil {
		log.Fatalf("error while creating telegram client: %v", err)
	}

	leetcodeClient := leetcode.New("https://leetcode.com")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := cron.New(cron.WithLocation(moscowLoc))

	_, err = c.AddFunc("0 11 * * *", func() {
		log.Printf("Запуск ежедневной задачи в %s...", time.Now().In(moscowLoc).Format("15:04"))
		sendDailyPost(ctx, leetcodeClient, tgClient.SendToAll)
	})
	if err != nil {
		log.Fatal("Ошибка настройки cron:", err)
	}

	// The start-up post goes to the fixed chats only, so a restart does not
	// send subscribers a second copy of the same problem.
	sendDailyPost(ctx, leetcodeClient, tgClient.SendToFixedChats)

	c.Start()
	log.Println("Планировщик запущен. Ожидание задач...")

	tgBot.Start(ctx)

	<-c.Stop().Done()
	log.Println("Планировщик остановлен")
}

// sendDailyPost fetches today's problem and hands it to send. A failure is
// logged and the service keeps running, so the next day still gets a post.
func sendDailyPost(ctx context.Context, leetcodeClient leetcode.Client, send func(context.Context, domain.DailyQuestion)) {
	res, err := leetcodeClient.GetCurrentDailyTask(ctx)
	if err != nil {
		log.Printf("error while getting daily task: %v", err)
		return
	}

	send(ctx, res.Result.ActiveDailyCodingChallengeQuestion)
}
