// Package daily fetches the LeetCode daily problem and hands it to Telegram.
package daily

import (
	"context"
	"log"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/leetcode"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

// Sender delivers the post for one Moscow hour.
type Sender interface {
	HasRecipients(hour int) bool
	SendForHour(ctx context.Context, hour int, question domain.DailyQuestion)
}

// SendWindow runs one delivery window, the Moscow hour that just started. It
// fetches the current problem once for the whole window, and not at all when
// nobody is due, so LeetCode is asked at most once an hour.
func SendWindow(ctx context.Context, leetcodeClient leetcode.Client, sender Sender, hour int) {
	if !sender.HasRecipients(hour) {
		return
	}

	Post(ctx, leetcodeClient, func(ctx context.Context, question domain.DailyQuestion) {
		sender.SendForHour(ctx, hour, question)
	})
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
