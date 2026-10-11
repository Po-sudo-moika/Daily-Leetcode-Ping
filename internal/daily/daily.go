// Package daily fetches the LeetCode daily problem and hands it to Telegram.
package daily

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/leetcode"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

// Sender delivers the post for one Moscow hour.
type Sender interface {
	HasRecipients(hour int) bool
	SendForHour(ctx context.Context, hour int, question domain.DailyQuestion)
}

// Schedule serves the hourly delivery windows. It remembers the last hour it
// served today, so a run after the host slept through some windows serves
// all of them. Nothing is saved: after a restart it starts from the hour
// after start-up.
type Schedule struct {
	leetcode leetcode.Client
	sender   Sender

	mu sync.Mutex
	// served is a time in the last hour served.
	served time.Time
}

// NewSchedule takes the start-up time in Moscow time. Its hour counts as
// served, so a restart never re-sends an hour an earlier process may have
// served, and the first run catches up even if the host slept before it.
func NewSchedule(leetcodeClient leetcode.Client, sender Sender, start time.Time) *Schedule {
	return &Schedule{leetcode: leetcodeClient, sender: sender, served: start}
}

// Run serves every Moscow hour of today from the first one not yet served
// through the hour of now, each once. now must be in Moscow time. It fetches
// the current problem once for the whole run, and not at all when nobody is
// due, so LeetCode is asked at most once an hour.
func (s *Schedule) Run(ctx context.Context, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	from := 0
	if sameDay(s.served, now) {
		from = s.served.Hour() + 1
	}
	if from > now.Hour() {
		return
	}
	s.served = now

	var due []int
	for hour := from; hour <= now.Hour(); hour++ {
		if s.sender.HasRecipients(hour) {
			due = append(due, hour)
		}
	}
	if len(due) == 0 {
		return
	}

	Post(ctx, s.leetcode, func(ctx context.Context, question domain.DailyQuestion) {
		for _, hour := range due {
			s.sender.SendForHour(ctx, hour, question)
		}
	})
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()

	return ay == by && am == bm && ad == bd
}

// Post fetches today's problem and hands it to send. A failure is logged and
// the service keeps running, so the next window still gets a post.
func Post(ctx context.Context, leetcodeClient leetcode.Client, send func(context.Context, domain.DailyQuestion)) {
	res, err := leetcodeClient.GetCurrentDailyTask(ctx)
	if err != nil {
		log.Printf("error while getting daily task: %v", err)
		return
	}

	send(ctx, res.Result.ActiveDailyCodingChallengeQuestion)
}
