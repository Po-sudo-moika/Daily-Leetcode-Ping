package daily

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/telegram"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type fakeLeetcode struct {
	calls int
	err   error
	title string
}

func (f *fakeLeetcode) GetCurrentDailyTask(context.Context) (domain.Task, error) {
	f.calls++
	if f.err != nil {
		return domain.Task{}, f.err
	}

	return domain.Task{Result: domain.Data{ActiveDailyCodingChallengeQuestion: domain.DailyQuestion{
		Quest: domain.Question{Title: f.title},
	}}}, nil
}

// fakeSender has the chats in due waiting for the post until SendDue
// delivers it to them, like the Telegram client.
type fakeSender struct {
	due  []string
	sent []string
	got  []string
	at   []time.Time
}

func (f *fakeSender) HasDue(time.Time) bool { return len(f.due) > 0 }

func (f *fakeSender) SendDue(_ context.Context, now time.Time, q domain.DailyQuestion) error {
	f.sent = append(f.sent, f.due...)
	f.due = nil
	f.got = append(f.got, q.Quest.Title)
	f.at = append(f.at, now)

	return nil
}

var round = time.Date(2026, time.October, 11, 14, 10, 0, 0, time.FixedZone("MSK", 3*60*60))

func TestDeliver_FetchesOnceAndSendsToEveryoneDue(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	sender := &fakeSender{due: []string{"5", "@news"}}

	Deliver(context.Background(), lc, sender, round)

	if lc.calls != 1 {
		t.Fatalf("fetched %d times, want 1 for the round", lc.calls)
	}
	if !slices.Equal(sender.sent, []string{"5", "@news"}) || !slices.Equal(sender.got, []string{"Two Sum"}) {
		t.Fatalf("sent to %v with %v, want both chats with the fetched problem", sender.sent, sender.got)
	}
	if !slices.Equal(sender.at, []time.Time{round}) {
		t.Fatalf("sent for %v, want the round's time", sender.at)
	}
}

func TestDeliver_NobodyDueMeansNoFetch(t *testing.T) {
	lc := &fakeLeetcode{}
	sender := &fakeSender{}

	Deliver(context.Background(), lc, sender, round)

	if lc.calls != 0 || len(sender.got) != 0 {
		t.Fatalf("calls=%d sends=%v, want nothing", lc.calls, sender.got)
	}
}

// LeetCode is down on the wake-up round: nobody is sent or recorded, so the
// next round delivers to the same chats.
func TestDeliver_FetchFailureRecordsNothing(t *testing.T) {
	lc := &fakeLeetcode{err: errors.New("leetcode is down"), title: "Two Sum"}
	sender := &fakeSender{due: []string{"5", "@news"}}

	Deliver(context.Background(), lc, sender, round)
	if len(sender.sent) != 0 || !slices.Equal(sender.due, []string{"5", "@news"}) {
		t.Fatalf("after a failed fetch: sent %v, still due %v", sender.sent, sender.due)
	}

	lc.err = nil
	Deliver(context.Background(), lc, sender, round.Add(50*time.Minute))
	if !slices.Equal(sender.sent, []string{"5", "@news"}) {
		t.Fatalf("next round sent to %v, want [5 @news]", sender.sent)
	}
}

// telegramAPI stands in for Telegram under the real client and records the
// chats it sends to. The client uses no other call in a round.
type telegramAPI struct {
	telegram.API
	sent []string
}

func (a *telegramAPI) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	a.sent = append(a.sent, fmt.Sprint(params.ChatID))

	return &models.Message{}, nil
}

// LeetCode is down from 10:55, when one subscriber is due, until 11:00. The
// failed fetches must not hold back the 11:00 round: once LeetCode answers,
// the 10:55 subscriber, the fixed chat and the 11:00 subscriber get the post.
func TestDeliver_FailedFetchDoesNotDelayLaterTimes(t *testing.T) {
	store, err := subscribers.Open(filepath.Join(t.TempDir(), "subscribers.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, chatID := range []int64{1, 2} {
		if _, err := store.Add(chatID); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetTime(1, subscribers.At(10, 55)); err != nil {
		t.Fatal(err)
	}
	api := &telegramAPI{}
	sender, err := telegram.New(api, []string{"5"}, store)
	if err != nil {
		t.Fatal(err)
	}
	lc := &fakeLeetcode{err: errors.New("leetcode timeout"), title: "Two Sum"}
	at := func(hour, minute int) time.Time {
		return time.Date(round.Year(), round.Month(), round.Day(), hour, minute, 0, 0, round.Location())
	}

	for minute := 55; minute < 60; minute++ {
		Deliver(context.Background(), lc, sender, at(10, minute))
	}
	if len(api.sent) != 0 {
		t.Fatalf("sent to %v while LeetCode was down", api.sent)
	}

	lc.err = nil
	Deliver(context.Background(), lc, sender, at(11, 0))
	slices.Sort(api.sent)
	if want := []string{"1", "2", "5"}; !slices.Equal(api.sent, want) {
		t.Fatalf("the 11:00 round sent to %v, want %v", api.sent, want)
	}
}

func TestPost_SendsFetchedProblem(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	var got string

	Post(context.Background(), lc, func(_ context.Context, q domain.DailyQuestion) error {
		got = q.Quest.Title
		return errors.New("send failed, only logged")
	})

	if got != "Two Sum" {
		t.Fatalf("got %q", got)
	}
}
