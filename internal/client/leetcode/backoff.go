package leetcode

import (
	"context"
	"sync"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

// Backoff wraps a Client so that a failed fetch is not repeated for wait: the
// delivery round runs every minute, and a LeetCode that is down should not be
// asked every minute. Within wait it returns the error of the failed fetch
// without calling LeetCode. A successful fetch clears it.
type Backoff struct {
	next Client
	wait time.Duration
	now  func() time.Time

	mu        sync.Mutex
	failedAt  time.Time
	failedErr error
}

// NewBackoff returns next with a pause of wait after each failed fetch.
func NewBackoff(next Client, wait time.Duration) *Backoff {
	return &Backoff{next: next, wait: wait, now: time.Now}
}

func (b *Backoff) GetCurrentDailyTask(ctx context.Context) (domain.Task, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.failedErr != nil && b.now().Sub(b.failedAt) < b.wait {
		return domain.Task{}, b.failedErr
	}

	task, err := b.next.GetCurrentDailyTask(ctx)
	if err != nil {
		b.failedAt, b.failedErr = b.now(), err
		return domain.Task{}, err
	}

	b.failedErr = nil

	return task, nil
}
