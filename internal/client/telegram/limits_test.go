package telegram

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/leetcode"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/links"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestRateLimit_DropsExtraActionsAndSaysSlowDownOnce(t *testing.T) {
	ctx := context.Background()
	subs := subscribed(10)

	limits := NewLimits()
	now := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)
	limits.now = func() time.Time { return now }

	b, err := bot.New("123456:fake-token-for-tests", append(BotOptions(subs, nil, limits), bot.WithSkipGetMe())...)
	if err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{}
	NewMenu(api, subs, nil).WithLimits(limits).Register(b)

	// The first userBurst commands pass; the five after them are dropped.
	for range userBurst + 5 {
		b.ProcessUpdate(ctx, textUpdate(10, models.ChatTypePrivate, 10, "", "/settings"))
	}

	if len(api.sent) != userBurst+1 {
		t.Fatalf("got %d messages, want %d answers and one notice", len(api.sent), userBurst)
	}
	if notice := api.sent[userBurst]; notice.Text != tooManyText.in(subscribers.Russian) || notice.ChatID != int64(10) {
		t.Fatalf("the last message is %+v, want the notice to chat 10", notice)
	}

	// A button press over the limit is dropped too, and still answered, so
	// Telegram shows no spinner. The notice was sent in this window already.
	press := callbackUpdate(10, callbackAbout)
	press.CallbackQuery.From = models.User{ID: 10}
	b.ProcessUpdate(ctx, press)

	if len(api.edited) != 0 || len(api.answered) != 1 || api.answered[0].Text != "" {
		t.Fatalf("a dropped press must only be answered: edited %d, answered %+v", len(api.edited), api.answered)
	}
	if len(api.sent) != userBurst+1 {
		t.Fatalf("a second notice was sent in the same window: %d messages", len(api.sent))
	}
}

func TestLookups_SameUserTwiceMakesOneCallAndInvalidNameNone(t *testing.T) {
	ctx := context.Background()
	subs := subscribed()

	store, err := links.Open(filepath.Join(t.TempDir(), "links.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(10, "alice_tg", "Alice"); err != nil {
		t.Fatal(err)
	}
	// A name the bot would never accept, as if the links file was edited by hand.
	if err := store.Set(11, "mallory_tg", "bad name!"); err != nil {
		t.Fatal(err)
	}

	lc := &fakeLeetCode{calendars: map[string]domain.Calendar{"Alice": {Username: "Alice"}}}
	b, err := bot.New("123456:fake-token-for-tests", append(BotOptions(subs, store, nil), bot.WithSkipGetMe())...)
	if err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{}
	menu := NewMenu(api, subs, nil).WithLeetCode(store, leetcode.NewGuarded(lc)).WithBotUsername(botName)
	menu.now = func() time.Time { return time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC) }
	menu.Register(b)

	b.ProcessUpdate(ctx, textUpdate(20, models.ChatTypePrivate, 20, "bob_tg", "/show @alice_tg"))
	b.ProcessUpdate(ctx, textUpdate(20, models.ChatTypePrivate, 20, "bob_tg", "/show @alice_tg"))

	if len(lc.asked) != 1 || len(api.sent) != 2 || api.sent[0].Text != api.sent[1].Text {
		t.Fatalf("LeetCode was asked %v, messages %+v; want one call and the same answer twice", lc.asked, api.sent)
	}

	b.ProcessUpdate(ctx, textUpdate(20, models.ChatTypePrivate, 20, "bob_tg", "/show @mallory_tg"))
	b.ProcessUpdate(ctx, textUpdate(20, models.ChatTypePrivate, 20, "bob_tg", "/leetcode not-valid!"))

	if len(lc.asked) != 1 {
		t.Fatalf("an invalid username reached LeetCode: %v", lc.asked)
	}
}
