package telegram

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func newMenu(subs *fakeSubscribers) (*Menu, *fakeAPI) {
	api := &fakeAPI{}
	return NewMenu(api, subs), api
}

func buttons(t *testing.T, markup any) []models.InlineKeyboardButton {
	t.Helper()

	kb, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("reply markup is %T, want *models.InlineKeyboardMarkup", markup)
	}

	var out []models.InlineKeyboardButton
	for _, row := range kb.InlineKeyboard {
		out = append(out, row...)
	}

	return out
}

func TestHandleStart_ShowsWelcomeAndMenu(t *testing.T) {
	menu, api := newMenu(&fakeSubscribers{})

	menu.HandleStart(context.Background(), nil, startUpdate(10))

	if len(api.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(api.sent))
	}
	msg := api.sent[0]
	if msg.ChatID != int64(10) {
		t.Fatalf("sent to %v, want 10", msg.ChatID)
	}
	if msg.Text != welcomeText {
		t.Fatalf("text %q, want welcome text", msg.Text)
	}

	got := map[string]string{}
	for _, b := range buttons(t, msg.ReplyMarkup) {
		got[b.CallbackData] = b.Text
	}
	for _, data := range []string{callbackSubscribe, callbackUnsubscribe, callbackAbout} {
		if got[data] == "" {
			t.Errorf("menu has no button with callback %q: %v", data, got)
		}
	}
}

func TestHandleStart_IgnoresUpdateWithoutMessage(t *testing.T) {
	menu, api := newMenu(&fakeSubscribers{})

	menu.HandleStart(context.Background(), nil, &models.Update{})

	if len(api.sent) != 0 {
		t.Fatalf("sent %d messages, want 0", len(api.sent))
	}
}

func TestHandleSubscribe(t *testing.T) {
	subs := &fakeSubscribers{}
	menu, api := newMenu(subs)

	menu.HandleSubscribe(context.Background(), nil, callbackUpdate(10, callbackSubscribe))

	if !slices.Equal(subs.chats, []int64{10}) {
		t.Fatalf("subscribers %v, want [10]", subs.chats)
	}
	if len(api.answered) != 1 || api.answered[0].CallbackQueryID != "cb-1" {
		t.Fatalf("callback not answered: %v", api.answered)
	}
	if len(api.sent) != 1 || api.sent[0].Text != subscribedText || api.sent[0].ChatID != int64(10) {
		t.Fatalf("unexpected reply: %+v", api.sent)
	}
	if len(buttons(t, api.sent[0].ReplyMarkup)) == 0 {
		t.Fatal("reply must keep the menu")
	}
}

func TestHandleSubscribe_TwiceKeepsOneSubscription(t *testing.T) {
	subs := &fakeSubscribers{}
	menu, api := newMenu(subs)

	menu.HandleSubscribe(context.Background(), nil, callbackUpdate(10, callbackSubscribe))
	menu.HandleSubscribe(context.Background(), nil, callbackUpdate(10, callbackSubscribe))

	if !slices.Equal(subs.chats, []int64{10}) {
		t.Fatalf("subscribers %v, want [10]", subs.chats)
	}
	if got := api.sent[1].Text; got != subscribedText {
		t.Fatalf("second reply %q, want the same state text", got)
	}
}

func TestHandleUnsubscribe(t *testing.T) {
	subs := &fakeSubscribers{chats: []int64{10, 20}}
	menu, api := newMenu(subs)

	menu.HandleUnsubscribe(context.Background(), nil, callbackUpdate(10, callbackUnsubscribe))

	if !slices.Equal(subs.chats, []int64{20}) {
		t.Fatalf("subscribers %v, want [20]", subs.chats)
	}
	if len(api.answered) != 1 {
		t.Fatalf("callback not answered: %v", api.answered)
	}
	if len(api.sent) != 1 || api.sent[0].Text != unsubscribedText || api.sent[0].ChatID != int64(10) {
		t.Fatalf("unexpected reply: %+v", api.sent)
	}
}

func TestHandleUnsubscribe_WhenNotSubscribed(t *testing.T) {
	menu, api := newMenu(&fakeSubscribers{})

	menu.HandleUnsubscribe(context.Background(), nil, callbackUpdate(10, callbackUnsubscribe))

	if len(api.sent) != 1 || api.sent[0].Text != unsubscribedText {
		t.Fatalf("unexpected reply: %+v", api.sent)
	}
}

func TestSubscribeUnsubscribe_ReportStoreFailure(t *testing.T) {
	handlers := map[string]func(*Menu) func(context.Context, *models.Update){
		"subscribe": func(m *Menu) func(context.Context, *models.Update) {
			return func(ctx context.Context, u *models.Update) { m.HandleSubscribe(ctx, nil, u) }
		},
		"unsubscribe": func(m *Menu) func(context.Context, *models.Update) {
			return func(ctx context.Context, u *models.Update) { m.HandleUnsubscribe(ctx, nil, u) }
		},
	}

	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			menu, api := newMenu(&fakeSubscribers{failWrite: errors.New("disk full")})

			handler(menu)(context.Background(), callbackUpdate(10, name))

			if len(api.sent) != 1 || api.sent[0].Text != saveFailedText {
				t.Fatalf("unexpected reply: %+v", api.sent)
			}
		})
	}
}

func TestHandleAbout_DescribesProjectAndLinksToGitHub(t *testing.T) {
	subs := &fakeSubscribers{}
	menu, api := newMenu(subs)

	menu.HandleAbout(context.Background(), nil, callbackUpdate(10, callbackAbout))

	if len(api.answered) != 1 {
		t.Fatalf("callback not answered: %v", api.answered)
	}
	if len(api.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(api.sent))
	}
	if api.sent[0].Text != aboutText || !strings.Contains(aboutText, "LeetCode") {
		t.Fatalf("unexpected about text: %q", api.sent[0].Text)
	}

	var urls []string
	for _, b := range buttons(t, api.sent[0].ReplyMarkup) {
		if b.URL != "" {
			urls = append(urls, b.URL)
		}
	}
	want := "https://github.com/Po-sudo-moika/Daily-Leetcode-Ping"
	if !slices.Equal(urls, []string{want}) {
		t.Fatalf("link buttons %v, want [%s]", urls, want)
	}
	if len(subs.chats) != 0 {
		t.Fatalf("about must not change subscriptions: %v", subs.chats)
	}
}

func TestCallbackHandlers_IgnoreInaccessibleMessage(t *testing.T) {
	subs := &fakeSubscribers{}
	menu, api := newMenu(subs)
	update := &models.Update{CallbackQuery: &models.CallbackQuery{ID: "cb-1", Data: callbackSubscribe}}

	menu.HandleSubscribe(context.Background(), nil, update)

	if len(subs.chats) != 0 || len(api.sent) != 0 {
		t.Fatalf("nothing should happen: subs=%v sent=%v", subs.chats, api.sent)
	}
	if len(api.answered) != 1 {
		t.Fatal("the button spinner must still be stopped")
	}
}
