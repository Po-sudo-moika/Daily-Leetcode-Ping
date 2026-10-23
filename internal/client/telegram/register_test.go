package telegram

import (
	"context"
	"slices"
	"testing"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// These tests send updates through a real bot, to prove that Register wires
// each update to the right handler.

func newRegisteredBot(t *testing.T, subs Subscribers, fixedChats ...string) (*bot.Bot, *fakeAPI) {
	t.Helper()

	return newRegisteredBotWithOwner(t, subs, 0, fixedChats...)
}

func newRegisteredBotWithOwner(t *testing.T, subs Subscribers, ownerID int64, fixedChats ...string) (*bot.Bot, *fakeAPI) {
	t.Helper()

	options := append(BotOptions(subs), bot.WithSkipGetMe(), bot.WithNotAsyncHandlers())
	b, err := bot.New("123456:fake-token-for-tests", options...)
	if err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{}
	NewMenu(api, subs, fixedChats).WithOwner(ownerID).Register(b)

	return b, api
}

func commandUpdate(chatID int64, text string) *models.Update {
	update := startUpdate(chatID)
	update.Message.Text = text
	update.Message.Entities = []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: len(text)}}

	return update
}

func TestRegister_StartCommand(t *testing.T) {
	b, api := newRegisteredBot(t, subscribed())

	b.ProcessUpdate(context.Background(), commandUpdate(10, "/start"))

	if len(api.sent) != 1 || api.sent[0].Text != welcomeFor(subscribers.DefaultTime, true) {
		t.Fatalf("unexpected reply to /start: %+v", api.sent)
	}
}

func TestRegister_SettingsCommand(t *testing.T) {
	b, api := newRegisteredBot(t, subscribed(10))

	b.ProcessUpdate(context.Background(), commandUpdate(10, "/settings"))

	if len(api.sent) != 1 || len(markedHours(t, api.sent[0].ReplyMarkup)) != 1 {
		t.Fatalf("unexpected reply to /settings: %+v", api.sent)
	}
}

func TestRegister_Buttons(t *testing.T) {
	subs := subscribed()
	b, api := newRegisteredBot(t, subs)
	ctx := context.Background()

	b.ProcessUpdate(ctx, callbackUpdate(10, callbackSubscribe))
	if !slices.Equal(subs.chats(), []int64{10}) {
		t.Fatalf("subscribe button: subscribers %v, want [10]", subs.chats())
	}

	b.ProcessUpdate(ctx, callbackUpdate(10, callbackSettings))
	b.ProcessUpdate(ctx, callbackUpdate(10, callbackHourPrefix+"15"))
	if at, _ := subs.Time(10); at != subscribers.DefaultTime {
		t.Fatalf("hour button saved %s, want nothing saved yet", subscribers.Format(at))
	}
	b.ProcessUpdate(ctx, callbackUpdate(10, timeData(15, 45)))
	if at, _ := subs.Time(10); at != subscribers.At(15, 45) {
		t.Fatalf("minute button: time %s, want 15:45", subscribers.Format(at))
	}

	b.ProcessUpdate(ctx, callbackUpdate(10, callbackMenu))
	b.ProcessUpdate(ctx, callbackUpdate(10, callbackAbout))
	b.ProcessUpdate(ctx, callbackUpdate(10, callbackUnsubscribe))
	if len(subs.chats()) != 0 {
		t.Fatalf("unsubscribe button: subscribers %v, want none", subs.chats())
	}

	if len(api.sent) != 0 {
		t.Fatalf("buttons must edit, not send: %+v", api.sent)
	}
	var texts []string
	for _, e := range api.edited {
		texts = append(texts, e.Text)
	}
	if len(texts) != 7 || texts[0] != subscribedText(subscribers.DefaultTime) || texts[4] != welcomeFor(subscribers.At(15, 45), true) || texts[5] != aboutText || texts[6] != unsubscribedText {
		t.Fatalf("edited texts %q", texts)
	}
}
