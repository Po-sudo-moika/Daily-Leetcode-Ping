package daily

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
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

// fakeSender has recipients at the hours in due.
type fakeSender struct {
	due  []int
	sent []int
	got  []string
}

func (f *fakeSender) HasRecipients(hour int) bool { return slices.Contains(f.due, hour) }

func (f *fakeSender) SendForHour(_ context.Context, hour int, q domain.DailyQuestion) {
	f.sent = append(f.sent, hour)
	f.got = append(f.got, q.Quest.Title)
}

func TestSendWindow_FetchesOnceAndSendsForThatHour(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	sender := &fakeSender{due: []int{9, 11}}

	SendWindow(context.Background(), lc, sender, 9)

	if lc.calls != 1 {
		t.Fatalf("fetched %d times, want 1 for the window", lc.calls)
	}
	if !slices.Equal(sender.sent, []int{9}) || sender.got[0] != "Two Sum" {
		t.Fatalf("sent %v %v, want hour 9 with the fetched problem", sender.sent, sender.got)
	}
}

// A day of hourly windows asks LeetCode only for the hours somebody is due,
// and each time for the current problem.
func TestSendWindow_OneFetchPerBusyWindow(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	sender := &fakeSender{due: []int{4, 9, 11}}

	for hour := 0; hour < 24; hour++ {
		SendWindow(context.Background(), lc, sender, hour)
	}

	if lc.calls != 3 {
		t.Fatalf("fetched %d times, want 3 (one per hour with recipients)", lc.calls)
	}
	if !slices.Equal(sender.sent, []int{4, 9, 11}) {
		t.Fatalf("sent for hours %v, want [4 9 11]", sender.sent)
	}
}

func TestSendWindow_NoRecipientsMeansNoFetch(t *testing.T) {
	lc := &fakeLeetcode{}
	sender := &fakeSender{due: []int{9}}

	SendWindow(context.Background(), lc, sender, 10)

	if lc.calls != 0 || len(sender.sent) != 0 {
		t.Fatalf("calls=%d sent=%v, want nothing", lc.calls, sender.sent)
	}
}

func TestSendWindow_FetchFailureSendsNothing(t *testing.T) {
	lc := &fakeLeetcode{err: errors.New("leetcode is down")}
	sender := &fakeSender{due: []int{9}}

	SendWindow(context.Background(), lc, sender, 9)

	if len(sender.sent) != 0 {
		t.Fatalf("sent %v after a failed fetch", sender.sent)
	}
}

func TestPost_SendsFetchedProblem(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	var got string

	Post(context.Background(), lc, func(_ context.Context, q domain.DailyQuestion) {
		got = q.Quest.Title
	})

	if got != "Two Sum" {
		t.Fatalf("got %q", got)
	}
}
