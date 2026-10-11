package telegram

import (
	"context"
	"slices"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// These tests send updates through a real bot, to prove that Register wires
// each update to the right handler.

func newRegisteredBot(t *testing.T, subs *fakeSubscribers) (*bot.Bot, *fakeAPI) {
	t.Helper()

	b, err := bot.New("123456:fake-token-for-tests", bot.WithSkipGetMe(), bot.WithNotAsyncHandlers())
	if err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{}
	NewMenu(api, subs).Register(b)

	return b, api
}

func TestRegister_StartCommand(t *testing.T) {
	b, api := newRegisteredBot(t, &fakeSubscribers{})

	update := startUpdate(10)
	update.Message.Entities = []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 6}}
	b.ProcessUpdate(context.Background(), update)

	if len(api.sent) != 1 || api.sent[0].Text != welcomeText {
		t.Fatalf("unexpected reply to /start: %+v", api.sent)
	}
}

func TestRegister_Buttons(t *testing.T) {
	subs := &fakeSubscribers{}
	b, api := newRegisteredBot(t, subs)
	ctx := context.Background()

	b.ProcessUpdate(ctx, callbackUpdate(10, callbackSubscribe))
	if !slices.Equal(subs.chats, []int64{10}) {
		t.Fatalf("subscribe button: subscribers %v, want [10]", subs.chats)
	}

	b.ProcessUpdate(ctx, callbackUpdate(10, callbackAbout))
	b.ProcessUpdate(ctx, callbackUpdate(10, callbackUnsubscribe))
	if len(subs.chats) != 0 {
		t.Fatalf("unsubscribe button: subscribers %v, want none", subs.chats)
	}

	var texts []string
	for _, p := range api.sent {
		texts = append(texts, p.Text)
	}
	if want := []string{subscribedText, aboutText, unsubscribedText}; !slices.Equal(texts, want) {
		t.Fatalf("replies %q, want %q", texts, want)
	}
}
