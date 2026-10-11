package daily

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

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

var moscow = time.FixedZone("MSK", 3*60*60)

func at(day, hour, minute int) time.Time {
	return time.Date(2026, time.October, day, hour, minute, 0, 0, moscow)
}

func TestSchedule_FirstRunServesOnlyItsOwnHour(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	sender := &fakeSender{due: []int{9, 11}}

	NewSchedule(lc, sender).Run(context.Background(), at(11, 11, 0))

	if lc.calls != 1 {
		t.Fatalf("fetched %d times, want 1 for the run", lc.calls)
	}
	if !slices.Equal(sender.sent, []int{11}) || sender.got[0] != "Two Sum" {
		t.Fatalf("sent %v %v, want only hour 11 with the fetched problem", sender.sent, sender.got)
	}
}

// A day of hourly runs asks LeetCode only for the hours somebody is due,
// and each time for the current problem.
func TestSchedule_OneFetchPerBusyHour(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	sender := &fakeSender{due: []int{4, 9, 11}}
	s := NewSchedule(lc, sender)

	for hour := 0; hour < 24; hour++ {
		s.Run(context.Background(), at(11, hour, 0))
	}

	if lc.calls != 3 {
		t.Fatalf("fetched %d times, want 3 (one per hour with recipients)", lc.calls)
	}
	if !slices.Equal(sender.sent, []int{4, 9, 11}) {
		t.Fatalf("sent for hours %v, want [4 9 11]", sender.sent)
	}
}

// The host sleeps from 10:30 to 14:10: the run on wake-up serves 11 (the
// fixed chats' hour) through 14 with one fetch, and later runs go on from
// there, so every hour is served exactly once.
func TestSchedule_CatchesUpAfterHostSleeps(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	sender := &fakeSender{due: []int{9, 11, 12, 13, 14, 15}}
	s := NewSchedule(lc, sender)
	ctx := context.Background()

	s.Run(ctx, at(11, 9, 0))
	s.Run(ctx, at(11, 10, 0))
	s.Run(ctx, at(11, 14, 10))
	if !slices.Equal(sender.sent, []int{9, 11, 12, 13, 14}) || lc.calls != 2 {
		t.Fatalf("after wake-up: sent %v with %d fetches, want [9 11 12 13 14] with 2", sender.sent, lc.calls)
	}

	s.Run(ctx, at(11, 15, 0))
	if !slices.Equal(sender.sent, []int{9, 11, 12, 13, 14, 15}) || lc.calls != 3 {
		t.Fatalf("next hour: sent %v with %d fetches, want [9 11 12 13 14 15] with 3", sender.sent, lc.calls)
	}
}

func TestSchedule_SecondRunInSameHourSendsNothing(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	sender := &fakeSender{due: []int{11}}
	s := NewSchedule(lc, sender)

	s.Run(context.Background(), at(11, 11, 0))
	s.Run(context.Background(), at(11, 11, 30))

	if !slices.Equal(sender.sent, []int{11}) || lc.calls != 1 {
		t.Fatalf("sent %v with %d fetches, want [11] once", sender.sent, lc.calls)
	}
}

// Sleeping past midnight serves today's hours from 00:00, and not the hours
// missed yesterday: those subscribers get today's problem at their own hour.
func TestSchedule_NewDayStartsFromMidnight(t *testing.T) {
	lc := &fakeLeetcode{title: "Two Sum"}
	sender := &fakeSender{due: []int{9, 11, 23}}
	s := NewSchedule(lc, sender)

	s.Run(context.Background(), at(11, 22, 0))
	s.Run(context.Background(), at(12, 12, 5))

	if !slices.Equal(sender.sent, []int{9, 11}) || lc.calls != 1 {
		t.Fatalf("sent %v with %d fetches, want [9 11] with 1", sender.sent, lc.calls)
	}
}

func TestSchedule_NoRecipientsMeansNoFetch(t *testing.T) {
	lc := &fakeLeetcode{}
	sender := &fakeSender{due: []int{9}}

	NewSchedule(lc, sender).Run(context.Background(), at(11, 10, 0))

	if lc.calls != 0 || len(sender.sent) != 0 {
		t.Fatalf("calls=%d sent=%v, want nothing", lc.calls, sender.sent)
	}
}

func TestSchedule_FetchFailureSendsNothing(t *testing.T) {
	lc := &fakeLeetcode{err: errors.New("leetcode is down")}
	sender := &fakeSender{due: []int{9}}

	NewSchedule(lc, sender).Run(context.Background(), at(11, 9, 0))

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
