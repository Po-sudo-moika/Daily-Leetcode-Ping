package leetcode

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

type countingClient struct {
	calls int
	err   error
}

func (c *countingClient) GetCurrentDailyTask(context.Context) (domain.Task, error) {
	c.calls++

	return domain.Task{}, c.err
}

func TestBackoff_AFailedFetchIsNotRepeatedUntilTheWaitIsOver(t *testing.T) {
	inner := &countingClient{err: errors.New("leetcode is down")}
	now := time.Date(2026, time.October, 11, 9, 30, 0, 0, time.UTC)
	b := NewBackoff(inner, time.Hour)
	b.now = func() time.Time { return now }

	for minute := 0; minute < 60; minute++ {
		now = now.Add(time.Minute)
		if _, err := b.GetCurrentDailyTask(context.Background()); err == nil {
			t.Fatal("want the error of the failed fetch")
		}
	}
	if inner.calls != 1 {
		t.Fatalf("LeetCode asked %d times within the wait, want 1", inner.calls)
	}

	now = now.Add(time.Minute)
	b.GetCurrentDailyTask(context.Background())
	if inner.calls != 2 {
		t.Fatalf("LeetCode asked %d times after the wait, want 2", inner.calls)
	}
}

func TestBackoff_ASuccessfulFetchIsNotHeldBack(t *testing.T) {
	inner := &countingClient{}
	b := NewBackoff(inner, time.Hour)

	for i := 0; i < 3; i++ {
		if _, err := b.GetCurrentDailyTask(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if inner.calls != 3 {
		t.Fatalf("LeetCode asked %d times, want 3", inner.calls)
	}
}
