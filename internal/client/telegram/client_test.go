package telegram

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
)

var question = domain.DailyQuestion{
	Date: "2026-10-11",
	Link: "/problems/two-sum/",
	Quest: domain.Question{
		Title:      "Two Sum",
		Difficulty: "Easy",
		AcRate:     55.5,
	},
}

func TestNew_Validates(t *testing.T) {
	if _, err := New(nil, nil, subscribed()); err == nil {
		t.Error("want an error for a nil api")
	}
	if _, err := New(&fakeAPI{}, nil, nil); err == nil {
		t.Error("want an error for nil subscribers")
	}
}

func TestSendToFixedChats_SkipsSubscribers(t *testing.T) {
	api := &fakeAPI{}
	c, err := New(api, []string{"111", "@news"}, subscribed(5))
	if err != nil {
		t.Fatal(err)
	}

	if err := c.SendToFixedChats(context.Background(), at(11, 10, 30), question); err != nil {
		t.Fatal(err)
	}

	if want := []any{"111", "@news"}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

// A start-up before 11:00 leaves the fixed chats' 11:00 post due as before.
func TestSendToFixedChats_BeforeFixedHourKeepsTodaysPostDue(t *testing.T) {
	api := &fakeAPI{}
	c, _ := New(api, []string{"111", "@news"}, subscribed())

	if err := c.SendToFixedChats(context.Background(), at(11, 10, 30), question); err != nil {
		t.Fatal(err)
	}

	if got, want := round(t, c, api, at(11, 11, 0)), []any{"111", "@news"}; !slices.Equal(got, want) {
		t.Fatalf("11:00 sent to %v, want %v", got, want)
	}
}

// A start-up after 11:00 counts as the fixed chats' post for the day, except
// for a chat Telegram did not accept it for: the next round sends to that one.
func TestSendToFixedChats_AfterFixedHourIsNotRepeated(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{"@news": context.DeadlineExceeded}}
	c, _ := New(api, []string{"111", "@news"}, subscribed())

	if err := c.SendToFixedChats(context.Background(), at(11, 14, 30), question); err != nil {
		t.Fatal(err)
	}

	api.sendErr = nil
	if got, want := round(t, c, api, at(11, 15, 0)), []any{"@news"}; !slices.Equal(got, want) {
		t.Fatalf("15:00 sent to %v, want %v", got, want)
	}
}

// A fixed chat that is also a subscriber at 09:00 and gets the start-up post
// at 10:15 already had its post for the day: 11:00 does not repeat it.
func TestSendToFixedChats_SubscribedChatAfterItsHourIsNotRepeated(t *testing.T) {
	subs := subscribed(5)
	subs.set(5, 9)
	api := &fakeAPI{}
	c, _ := New(api, []string{"5", "@news"}, subs)

	if err := c.SendToFixedChats(context.Background(), at(11, 10, 15), question); err != nil {
		t.Fatal(err)
	}

	if got, want := round(t, c, api, at(11, 11, 0)), []any{"@news"}; !slices.Equal(got, want) {
		t.Fatalf("11:00 sent to %v, want %v", got, want)
	}
}

// A fixed chat that is also a subscriber at 18:00 and gets the start-up post
// at 14:30 still gets its post at 18:00, the hour it chose.
func TestSendToFixedChats_SubscribedChatBeforeItsHourKeepsItsPostDue(t *testing.T) {
	subs := subscribed(5)
	subs.set(5, 18)
	api := &fakeAPI{}
	c, _ := New(api, []string{"5"}, subs)

	if err := c.SendToFixedChats(context.Background(), at(11, 14, 30), question); err != nil {
		t.Fatal(err)
	}

	if got := round(t, c, api, at(11, 15, 0)); len(got) != 0 {
		t.Fatalf("15:00 sent to %v, want nobody", got)
	}
	if got, want := round(t, c, api, at(11, 18, 0)), []any{int64(5)}; !slices.Equal(got, want) {
		t.Fatalf("18:00 sent to %v, want %v", got, want)
	}
}

var msk = time.FixedZone("MSK", 3*60*60)

// at is a Moscow time on a day of October 2026.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, time.October, day, hour, minute, 0, 0, msk)
}

func TestSendDue_FixedChatsAndSubscribersAtDefaultHour(t *testing.T) {
	api := &fakeAPI{}
	c, err := New(api, []string{"111"}, subscribed(5, 6))
	if err != nil {
		t.Fatal(err)
	}

	if err := c.SendDue(context.Background(), at(11, subscribers.DefaultHour, 0), question); err != nil {
		t.Fatal(err)
	}

	if want := []any{"111", int64(5), int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
	for _, p := range api.sent {
		if p.Text != getPostMessageSchema(question) {
			t.Fatalf("unexpected text %q", p.Text)
		}
	}
}

// Each chat is due once its own hour has come, and only until it got the post.
func TestSendDue_OnlyChatsWhoseHourHasCome(t *testing.T) {
	subs := subscribed(1, 2, 3)
	subs.set(1, 9)
	subs.set(3, 9)
	api := &fakeAPI{}
	c, _ := New(api, []string{"111"}, subs)
	ctx := context.Background()

	if err := c.SendDue(ctx, at(11, 9, 0), question); err != nil {
		t.Fatal(err)
	}
	if want := []any{int64(1), int64(3)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("at 09:00 sent to %v, want %v (the others wait for 11:00)", api.recipients(), want)
	}

	api.sent = nil
	if err := c.SendDue(ctx, at(11, 11, 0), question); err != nil {
		t.Fatal(err)
	}
	if want := []any{"111", int64(2)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("at 11:00 sent to %v, want %v", api.recipients(), want)
	}

	api.sent = nil
	if err := c.SendDue(ctx, at(11, 15, 0), question); err != nil {
		t.Fatal(err)
	}
	if len(api.sent) != 0 {
		t.Fatalf("at 15:00 everyone already got the post, sent to %v", api.recipients())
	}
}

func TestSendDue_EveryChatGetsOnePostADay(t *testing.T) {
	subs := subscribed(1, 2, 3, 4)
	subs.set(1, 4)
	subs.set(2, 9)
	subs.set(4, 23)
	api := &fakeAPI{}
	c, _ := New(api, []string{"111"}, subs)

	for day := 11; day <= 12; day++ {
		for hour := 0; hour < 24; hour++ {
			if err := c.SendDue(context.Background(), at(day, hour, 0), question); err != nil {
				t.Fatal(err)
			}
		}
	}

	oneDay := []any{int64(1), int64(2), "111", int64(3), int64(4)}
	if got, want := api.recipients(), append(slices.Clone(oneDay), oneDay...); !slices.Equal(got, want) {
		t.Fatalf("two days of rounds sent to %v, want %v", got, want)
	}
}

func TestHasDue(t *testing.T) {
	subs := subscribed(1)
	subs.set(1, 9)
	c, _ := New(&fakeAPI{}, []string{"111"}, subs)
	ctx := context.Background()

	for now, want := range map[time.Time]bool{at(11, 4, 0): false, at(11, 8, 59): false, at(11, 9, 0): true, at(11, 10, 0): true} {
		if got := c.HasDue(now); got != want {
			t.Errorf("HasDue(%s) = %v, want %v", now.Format("15:04"), got, want)
		}
	}

	if err := c.SendDue(ctx, at(11, 10, 0), question); err != nil {
		t.Fatal(err)
	}
	if c.HasDue(at(11, 10, 30)) {
		t.Error("nobody is due after the 10:00 round")
	}
	if !c.HasDue(at(11, 11, 0)) {
		t.Error("the fixed chat is due at 11:00")
	}

	noFixed, _ := New(&fakeAPI{}, nil, subs)
	if noFixed.HasDue(at(11, 11, 0)) {
		t.Error("nobody is due at 11:00 without fixed chats")
	}
}

// A fixed chat that is also a subscriber gets the post once a day, at the
// hour it chose, not at both the chosen and the fixed hour.
func TestSendDue_FixedChatThatSubscribedFollowsItsHour(t *testing.T) {
	subs := subscribed(5, 6)
	subs.set(5, 9)
	api := &fakeAPI{}
	c, _ := New(api, []string{"5", "7"}, subs)
	ctx := context.Background()

	if err := c.SendDue(ctx, at(11, 9, 0), question); err != nil {
		t.Fatal(err)
	}
	if err := c.SendDue(ctx, at(11, 11, 0), question); err != nil {
		t.Fatal(err)
	}

	if want := []any{int64(5), "7", int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendDue_NoDuplicateWhenSubscriberIsAFixedChatAtDefaultHour(t *testing.T) {
	api := &fakeAPI{}
	c, _ := New(api, []string{"5"}, subscribed(5, 6))

	if err := c.SendDue(context.Background(), at(11, 11, 0), question); err != nil {
		t.Fatal(err)
	}

	if want := []any{int64(5), int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendDue_DropsSubscriberWhoBlockedTheBot(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{int64(5): errBlocked}}
	subs := subscribed(5, 6)
	c, _ := New(api, nil, subs)

	if err := c.SendDue(context.Background(), at(11, 11, 0), question); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(subs.chats(), []int64{6}) {
		t.Fatalf("subscribers %v, want [6]", subs.chats())
	}
	if want := []any{int64(5), int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("one blocked user must not stop the others: sent to %v", api.recipients())
	}
}

// A send that failed is not recorded, so the next round tries that chat again
// and only that chat.
func TestSendDue_RetriesFailedChatNextRound(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{int64(5): context.DeadlineExceeded, "111": context.DeadlineExceeded}}
	subs := subscribed(5, 6)
	c, _ := New(api, []string{"111"}, subs)
	ctx := context.Background()

	if err := c.SendDue(ctx, at(11, 11, 0), question); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(subs.chats(), []int64{5, 6}) {
		t.Fatalf("a temporary error must keep the subscriber: %v", subs.chats())
	}

	api.sent, api.sendErr = nil, nil
	if err := c.SendDue(ctx, at(11, 12, 0), question); err != nil {
		t.Fatal(err)
	}
	if want := []any{"111", int64(5)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("next round sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendDue_KeepsFixedChatWhenForbidden(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{"111": errBlocked}}
	subs := subscribed(6)
	c, _ := New(api, []string{"111"}, subs)

	if err := c.SendDue(context.Background(), at(11, 11, 0), question); err != nil {
		t.Fatal(err)
	}

	if want := []any{"111", int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
	if !slices.Equal(subs.chats(), []int64{6}) {
		t.Fatalf("subscribers changed: %v", subs.chats())
	}
}

// openStore opens the real subscribers file at path with these chats at
// these hours.
func openStore(t *testing.T, path string, hours map[int64]int) *subscribers.Store {
	t.Helper()

	store, err := subscribers.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for chatID, hour := range hours {
		if _, err := store.Add(chatID); err != nil {
			t.Fatal(err)
		}
		if err := store.SetHour(chatID, hour); err != nil {
			t.Fatal(err)
		}
	}

	return store
}

// round runs one hourly round like the service does: a send only when
// somebody is due. It returns who got the post.
func round(t *testing.T, c Client, api *fakeAPI, now time.Time) []any {
	t.Helper()

	api.sent = nil
	if c.HasDue(now) {
		if err := c.SendDue(context.Background(), now, question); err != nil {
			t.Fatal(err)
		}
	}

	return api.recipients()
}

// The host sleeps from 08:50 to 14:10: the round on wake-up sends to every
// chat whose hour passed in between, each once.
func TestSendDue_CatchesUpAfterHostSleeps(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "subscribers.json"), map[int64]int{1: 9, 2: 11, 3: 13, 4: 15})
	api := &fakeAPI{}
	c, _ := New(api, []string{"@news"}, store)

	if got := round(t, c, api, at(11, 8, 0)); len(got) != 0 {
		t.Fatalf("08:00 sent to %v, want nobody", got)
	}
	if got, want := round(t, c, api, at(11, 14, 10)), []any{"@news", int64(1), int64(2), int64(3)}; !slices.Equal(got, want) {
		t.Fatalf("wake-up at 14:10 sent to %v, want %v", got, want)
	}
	if got, want := round(t, c, api, at(11, 15, 0)), []any{int64(4)}; !slices.Equal(got, want) {
		t.Fatalf("15:00 sent to %v, want %v", got, want)
	}
	if got := round(t, c, api, at(11, 16, 0)); len(got) != 0 {
		t.Fatalf("16:00 sent to %v, want nobody", got)
	}
}

// The service delivers at 11:00, is down from 11:30 to 13:30 and starts
// again: the deliveries are read back from the file, so nobody gets a second
// post and the hour it was down for is caught up.
func TestSendDue_RestartKeepsDeliveries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscribers.json")
	api := &fakeAPI{}
	before, _ := New(api, []string{"@news"}, openStore(t, path, map[int64]int{1: 11, 2: 12}))

	if got, want := round(t, before, api, at(11, 11, 0)), []any{"@news", int64(1)}; !slices.Equal(got, want) {
		t.Fatalf("11:00 sent to %v, want %v", got, want)
	}

	after, _ := New(api, []string{"@news"}, openStore(t, path, nil))
	if got, want := round(t, after, api, at(11, 14, 0)), []any{int64(2)}; !slices.Equal(got, want) {
		t.Fatalf("first round after the restart sent to %v, want %v", got, want)
	}
}

// The subscriber got the 11:00 post and then moves to 18:00: no second post
// that day, and the new hour from the next day.
func TestSendDue_HourChangeAfterDeliveryDoesNotResend(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "subscribers.json"), map[int64]int{1: 11})
	api := &fakeAPI{}
	c, _ := New(api, nil, store)

	if got := round(t, c, api, at(11, 11, 0)); !slices.Equal(got, []any{int64(1)}) {
		t.Fatalf("11:00 sent to %v, want [1]", got)
	}
	if err := store.SetHour(1, 18); err != nil {
		t.Fatal(err)
	}
	if got := round(t, c, api, at(11, 18, 0)); len(got) != 0 {
		t.Fatalf("18:00 sent to %v, want nobody: the post already went out today", got)
	}
	if got := round(t, c, api, at(12, 11, 0)); len(got) != 0 {
		t.Fatalf("next day 11:00 sent to %v, want nobody", got)
	}
	if got := round(t, c, api, at(12, 18, 0)); !slices.Equal(got, []any{int64(1)}) {
		t.Fatalf("next day 18:00 sent to %v, want [1]", got)
	}
}

// At 12:00 the subscriber moves from 18:00 to 09:00, which has passed: the
// next round sends today's post, and 18:00 sends nothing.
func TestSendDue_HourChangeToPassedHourDeliversNextRound(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "subscribers.json"), map[int64]int{1: 18})
	api := &fakeAPI{}
	c, _ := New(api, nil, store)

	if got := round(t, c, api, at(11, 12, 0)); len(got) != 0 {
		t.Fatalf("12:00 sent to %v, want nobody", got)
	}
	if err := store.SetHour(1, 9); err != nil {
		t.Fatal(err)
	}
	if got := round(t, c, api, at(11, 13, 0)); !slices.Equal(got, []any{int64(1)}) {
		t.Fatalf("13:00 sent to %v, want [1]", got)
	}
	if got := round(t, c, api, at(11, 18, 0)); len(got) != 0 {
		t.Fatalf("18:00 sent to %v, want nobody", got)
	}
	if got := round(t, c, api, at(12, 9, 0)); !slices.Equal(got, []any{int64(1)}) {
		t.Fatalf("next day 09:00 sent to %v, want [1]", got)
	}
}
