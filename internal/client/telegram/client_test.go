package telegram

import (
	"context"
	"slices"
	"testing"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
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
	if _, err := New(nil, nil, &fakeSubscribers{}); err == nil {
		t.Error("want an error for a nil api")
	}
	if _, err := New(&fakeAPI{}, nil, nil); err == nil {
		t.Error("want an error for nil subscribers")
	}
}

func TestSendToFixedChats_SkipsSubscribers(t *testing.T) {
	api := &fakeAPI{}
	c, err := New(api, []string{"111", "@news"}, &fakeSubscribers{chats: []int64{5}})
	if err != nil {
		t.Fatal(err)
	}

	c.SendToFixedChats(context.Background(), question)

	if want := []any{"111", "@news"}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendToAll_FixedChatsAndSubscribers(t *testing.T) {
	api := &fakeAPI{}
	c, err := New(api, []string{"111"}, &fakeSubscribers{chats: []int64{5, 6}})
	if err != nil {
		t.Fatal(err)
	}

	c.SendToAll(context.Background(), question)

	if want := []any{"111", int64(5), int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
	for _, p := range api.sent {
		if p.Text != getPostMessageSchema(question) {
			t.Fatalf("unexpected text %q", p.Text)
		}
	}
}

func TestSendToAll_NoFixedChats(t *testing.T) {
	api := &fakeAPI{}
	c, _ := New(api, nil, &fakeSubscribers{chats: []int64{5}})

	c.SendToAll(context.Background(), question)

	if want := []any{int64(5)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendToAll_NoDuplicateWhenSubscriberIsAFixedChat(t *testing.T) {
	api := &fakeAPI{}
	c, _ := New(api, []string{"5"}, &fakeSubscribers{chats: []int64{5, 6}})

	c.SendToAll(context.Background(), question)

	if want := []any{"5", int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
}

func TestSendToAll_DropsSubscriberWhoBlockedTheBot(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{int64(5): errBlocked}}
	subs := &fakeSubscribers{chats: []int64{5, 6}}
	c, _ := New(api, nil, subs)

	c.SendToAll(context.Background(), question)

	if !slices.Equal(subs.chats, []int64{6}) {
		t.Fatalf("subscribers %v, want [6]", subs.chats)
	}
	if want := []any{int64(5), int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("one blocked user must not stop the others: sent to %v", api.recipients())
	}
}

func TestSendToAll_KeepsSubscriberOnOtherErrors(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{int64(5): context.DeadlineExceeded}}
	subs := &fakeSubscribers{chats: []int64{5}}
	c, _ := New(api, nil, subs)

	c.SendToAll(context.Background(), question)

	if !slices.Equal(subs.chats, []int64{5}) {
		t.Fatalf("a temporary error must keep the subscriber: %v", subs.chats)
	}
}

func TestSendToAll_KeepsFixedChatWhenForbidden(t *testing.T) {
	api := &fakeAPI{sendErr: map[any]error{"111": errBlocked}}
	subs := &fakeSubscribers{chats: []int64{6}}
	c, _ := New(api, []string{"111"}, subs)

	c.SendToAll(context.Background(), question)

	if want := []any{"111", int64(6)}; !slices.Equal(api.recipients(), want) {
		t.Fatalf("sent to %v, want %v", api.recipients(), want)
	}
	if !slices.Equal(subs.chats, []int64{6}) {
		t.Fatalf("subscribers changed: %v", subs.chats)
	}
}
