package telegram

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/links"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const botName = "PingBot"

// fakeLeetCode knows the users in calendars and counts the questions.
type fakeLeetCode struct {
	calendars map[string]domain.Calendar
	err       error
	asked     []string
}

func (f *fakeLeetCode) GetUserCalendar(_ context.Context, username string) (domain.Calendar, error) {
	f.asked = append(f.asked, username)
	if f.err != nil {
		return domain.Calendar{}, f.err
	}
	for name, calendar := range f.calendars {
		if strings.EqualFold(name, username) {
			return calendar, nil
		}
	}

	return domain.Calendar{}, domain.ErrUserNotFound
}

// textUpdate is a message with a command at its start, written by user.
func textUpdate(chatID int64, chatType models.ChatType, user int64, login, text string) *models.Update {
	length := len(text)
	if i := strings.IndexByte(text, ' '); i >= 0 {
		length = i
	}

	return &models.Update{Message: &models.Message{
		Chat:     models.Chat{ID: chatID, Type: chatType},
		From:     &models.User{ID: user, Username: login},
		Text:     text,
		Entities: []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: length}},
	}}
}

// newLinkBot is a registered bot named botName with LeetCode linking wired to
// fakes. The clock stands at 2026-01-03 12:00 UTC.
func newLinkBot(t *testing.T, subs *fakeSubscribers, calendars map[string]domain.Calendar) (*bot.Bot, *fakeAPI, *links.Store, *fakeLeetCode) {
	t.Helper()

	store, err := links.Open(filepath.Join(t.TempDir(), "links.json"))
	if err != nil {
		t.Fatal(err)
	}
	lc := &fakeLeetCode{calendars: calendars}

	options := append(BotOptions(subs, store), bot.WithSkipGetMe(), bot.WithNotAsyncHandlers())
	b, err := bot.New("123456:fake-token-for-tests", options...)
	if err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{}
	menu := NewMenu(api, subs, nil).WithLeetCode(store, lc).WithBotUsername(botName)
	menu.now = func() time.Time { return time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC) }
	menu.Register(b)

	return b, api, store, lc
}

func utcDay(year int, month time.Month, day int) int64 {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix()
}

func TestLeetcodeCommand_LinksOnlyInAPrivateChat(t *testing.T) {
	ctx := context.Background()
	b, api, store, lc := newLinkBot(t, subscribed(), map[string]domain.Calendar{"Alice": {Username: "Alice"}})

	// A name LeetCode knows is saved, with the Telegram login of the sender.
	b.ProcessUpdate(ctx, textUpdate(10, models.ChatTypePrivate, 10, "alice_tg", "/leetcode Alice"))
	link, ok := store.Get(10)
	if !ok || link.LeetCode != "Alice" || link.Login != "alice_tg" {
		t.Fatalf("link of user 10 = %+v, %v; want Alice under alice_tg", link, ok)
	}
	if len(api.sent) != 1 || api.sent[0].Text != linkedText("Alice", subscribers.Russian) {
		t.Fatalf("reply to a known name: %+v", api.sent)
	}

	// A profile link works too.
	b.ProcessUpdate(ctx, textUpdate(11, models.ChatTypePrivate, 11, "bob_tg", "/leetcode https://leetcode.com/u/alice/"))
	if link, _ := store.Get(11); link.LeetCode != "Alice" {
		t.Fatalf("profile link saved %+v, want Alice", link)
	}

	// An unknown name is refused with a note and nothing is saved.
	api.sent = nil
	b.ProcessUpdate(ctx, textUpdate(12, models.ChatTypePrivate, 12, "carol_tg", "/leetcode nobody"))
	if _, ok := store.Get(12); ok {
		t.Fatal("an unknown name was saved")
	}
	if len(api.sent) != 1 || api.sent[0].Text != unknownUserText("nobody", subscribers.Russian) {
		t.Fatalf("reply to an unknown name: %+v", api.sent)
	}

	// In a group it only says that linking works in a private chat.
	api.sent, lc.asked = nil, nil
	b.ProcessUpdate(ctx, textUpdate(-100, models.ChatTypeGroup, 13, "dave_tg", "/leetcode Alice"))
	if _, ok := store.Get(13); ok || len(lc.asked) != 0 {
		t.Fatalf("a group linked a name or asked LeetCode: %v", lc.asked)
	}
	if len(api.sent) != 1 || api.sent[0].Text != privateOnlyText.in(subscribers.Russian) {
		t.Fatalf("reply in a group: %+v", api.sent)
	}
}

func TestLeetcodeCommand_AnswersInTheLanguageOfTheChat(t *testing.T) {
	subs := subscribed()
	_ = subs.SetLanguage(-100, subscribers.English)
	b, api, _, _ := newLinkBot(t, subs, nil)

	b.ProcessUpdate(context.Background(), textUpdate(-100, models.ChatTypeSupergroup, 13, "dave_tg", "/leetcode Alice"))

	if len(api.sent) != 1 || api.sent[0].Text != privateOnlyText.in(subscribers.English) || api.sent[0].Text == privateOnlyText.in(subscribers.Russian) {
		t.Fatalf("reply in an English group: %+v", api.sent)
	}
}

func TestShowCommand_AnswersWithTheStreakOfALinkedPerson(t *testing.T) {
	ctx := context.Background()
	// Submissions on 1, 2 and 3 January make three days; the clock says it is
	// the 3rd, and the run reaches back over New Year.
	calendars := map[string]domain.Calendar{"Alice": {Username: "Alice", BestStreak: 9, Submissions: map[int64]int{
		utcDay(2025, 12, 31): 1,
		utcDay(2026, 1, 1):   2,
		utcDay(2026, 1, 2):   1,
	}}}
	b, api, store, _ := newLinkBot(t, subscribed(), calendars)
	if err := store.Set(10, "Alice_TG", "Alice"); err != nil {
		t.Fatal(err)
	}

	// The case of the login does not matter, and a group can ask.
	b.ProcessUpdate(ctx, textUpdate(-100, models.ChatTypeGroup, 20, "dave_tg", "/show @alice_tg"))
	if len(api.sent) != 1 || api.sent[0].ChatID != int64(-100) || api.sent[0].Text != showText("Alice", 3, 9, subscribers.Russian) {
		t.Fatalf("reply to /show for a linked person: %+v", api.sent)
	}
	if !strings.Contains(api.sent[0].Text, "Alice") || !strings.Contains(api.sent[0].Text, "3") || !strings.Contains(api.sent[0].Text, "9") {
		t.Fatalf("reply %q lacks the name or the streaks", api.sent[0].Text)
	}

	// A login that has not linked gets the note.
	api.sent = nil
	b.ProcessUpdate(ctx, textUpdate(-100, models.ChatTypeGroup, 20, "dave_tg", "/show @nobody"))
	if len(api.sent) != 1 || api.sent[0].Text != notLinkedText("nobody", subscribers.Russian) {
		t.Fatalf("reply to /show for a login that has not linked: %+v", api.sent)
	}
}

func TestSettingsCommand_AddressedToThisBotOnly(t *testing.T) {
	ctx := context.Background()
	b, api, _, _ := newLinkBot(t, subscribed(-100), nil)

	b.ProcessUpdate(ctx, textUpdate(-100, models.ChatTypeGroup, 20, "dave_tg", "/settings@PingBot"))
	if len(api.sent) != 1 || len(markedHours(t, api.sent[0].ReplyMarkup)) != 1 {
		t.Fatalf("/settings@PingBot in a group: %+v", api.sent)
	}

	api.sent = nil
	b.ProcessUpdate(ctx, textUpdate(-100, models.ChatTypeGroup, 20, "dave_tg", "/settings@OtherBot"))
	if len(api.sent) != 0 {
		t.Fatalf("/settings@OtherBot got an answer: %+v", api.sent)
	}
}
