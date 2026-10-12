// Package daily fetches the LeetCode daily problem and hands it to Telegram.
package daily

import (
	"context"
	"log"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/leetcode"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

// Sender delivers the post to everyone who is due for it.
type Sender interface {
	HasDue(now time.Time) bool
	SendDue(ctx context.Context, now time.Time, question domain.DailyQuestion) error
}

// Deliver runs one delivery round at now, a Moscow time. Everyone whose hour
// has come and who has not got today's post gets it, so a round after the
// host slept or after a restart catches up on the hours it missed. It fetches
// the current problem once for the round, and not at all when nobody is due,
// so LeetCode is asked at most once an hour. A failed fetch sends nothing and
// records nothing, so the next round tries again.
func Deliver(ctx context.Context, leetcodeClient leetcode.Client, sender Sender, now time.Time) {
	if !sender.HasDue(now) {
		return
	}

	Post(ctx, leetcodeClient, func(ctx context.Context, question domain.DailyQuestion) error {
		return sender.SendDue(ctx, now, question)
	})
}

// Post fetches today's problem and hands it to send. A failure is logged and
// the service keeps running, so the next round still gets a post.
func Post(ctx context.Context, leetcodeClient leetcode.Client, send func(context.Context, domain.DailyQuestion) error) {
	res, err := leetcodeClient.GetCurrentDailyTask(ctx)
	if err != nil {
		log.Printf("error while getting daily task: %v", err)
		return
	}

	if err := send(ctx, res.Result.ActiveDailyCodingChallengeQuestion); err != nil {
		log.Printf("error while sending daily post: %v", err)
	}
}
