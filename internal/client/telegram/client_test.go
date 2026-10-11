package telegram

import (
	"context"
	"slices"
	"testing"

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

	c.SendToFixedChats(context.Background(), question)

	if want := []any{"111", "@news"}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendForHour_FixedChatsAndSubscribersAtDefaultHour(t *testing.T) {
	api := &fakeAPI{}
	c, err := New(api, []string{"111"}, subscribed(5, 6))
	if err != nil {
		t.Fatal(err)
	}

	c.SendForHour(context.Background(), subscribers.DefaultHour, question)

	if want := []any{"111", int64(5), int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
	for _, p := range api.sent {
		if p.Text != getPostMessageSchema(question) {
			t.Fatalf("unexpected text %q", p.Text)
		}
	}
}

// Each subscriber is served only in the window of their own hour.
func TestSendForHour_OnlyThatHoursSubscribers(t *testing.T) {
	subs := subscribed(1, 2, 3)
	subs.set(1, 9)
	subs.set(3, 9)
	api := &fakeAPI{}
	c, _ := New(api, []string{"111"}, subs)
	ctx := context.Background()

	c.SendForHour(ctx, 9, question)
	if want := []any{int64(1), int64(3)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("at 09:00 sent to %v, want %v (fixed chats wait for 11:00)", api.recipients(), want)
	}

	api.sent = nil
	c.SendForHour(ctx, 11, question)
	if want := []any{"111", int64(2)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("at 11:00 sent to %v, want %v", api.recipients(), want)
	}

	api.sent = nil
	c.SendForHour(ctx, 15, question)
	if len(api.sent) != 0 {
		t.Fatalf("at 15:00 nobody is due, sent to %v", api.recipients())
	}
}

func TestSendForHour_EverySubscriberGetsOnePostADay(t *testing.T) {
	subs := subscribed(1, 2, 3, 4)
	subs.set(1, 4)
	subs.set(2, 9)
	subs.set(4, 23)
	api := &fakeAPI{}
	c, _ := New(api, []string{"111"}, subs)

	for hour := 0; hour < 24; hour++ {
		c.SendForHour(context.Background(), hour, question)
	}

	want := []any{int64(1), int64(2), "111", int64(3), int64(4)}
	if got := api.recipients(); !slices.Equal(got, want) {
		t.Fatalf("a day of windows sent to %v, want %v", got, want)
	}
}

func TestHasRecipients(t *testing.T) {
	subs := subscribed(1)
	subs.set(1, 9)
	c, _ := New(&fakeAPI{}, []string{"111"}, subs)

	for hour, want := range map[int]bool{9: true, 11: true, 10: false, 4: false, 23: false} {
		if got := c.HasRecipients(hour); got != want {
			t.Errorf("HasRecipients(%d) = %v, want %v", hour, got, want)
		}
	}

	noFixed, _ := New(&fakeAPI{}, nil, subs)
	if noFixed.HasRecipients(11) {
		t.Error("nobody is due at 11:00 without fixed chats")
	}
}

// A fixed chat that is also a subscriber gets the post once a day, at the
// hour it chose, not at both the chosen and the fixed hour.
func TestSendForHour_FixedChatThatSubscribedFollowsItsHour(t *testing.T) {
	subs := subscribed(5, 6)
	subs.set(5, 9)
	api := &fakeAPI{}
	c, _ := New(api, []string{"5", "7"}, subs)
	ctx := context.Background()

	c.SendForHour(ctx, 9, question)
	c.SendForHour(ctx, 11, question)

	if want := []any{int64(5), "7", int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendForHour_NoDuplicateWhenSubscriberIsAFixedChatAtDefaultHour(t *testing.T) {
	api := &fakeAPI{}
	c, _ := New(api, []string{"5"}, subscribed(5, 6))

	c.SendForHour(context.Background(), 11, question)

	if want := []any{int64(5), int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendForHour_DropsSubscriberWhoBlockedTheBot(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{int64(5): errBlocked}}
	subs := subscribed(5, 6)
	c, _ := New(api, nil, subs)

	c.SendForHour(context.Background(), 11, question)

	if !slices.Equal(subs.chats(), []int64{6}) {
		t.Fatalf("subscribers %v, want [6]", subs.chats())
	}
	if want := []any{int64(5), int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("one blocked user must not stop the others: sent to %v", api.recipients())
	}
}

func TestSendForHour_KeepsSubscriberOnOtherErrors(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{int64(5): context.DeadlineExceeded}}
	subs := subscribed(5)
	c, _ := New(api, nil, subs)

	c.SendForHour(context.Background(), 11, question)

	if !slices.Equal(subs.chats(), []int64{5}) {
		t.Fatalf("a temporary error must keep the subscriber: %v", subs.chats())
	}
}

func TestSendForHour_KeepsFixedChatWhenForbidden(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{"111": errBlocked}}
	subs := subscribed(6)
	c, _ := New(api, []string{"111"}, subs)

	c.SendForHour(context.Background(), 11, question)

	if want := []any{"111", int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
	if !slices.Equal(subs.chats(), []int64{6}) {
		t.Fatalf("subscribers changed: %v", subs.chats())
	}
}
