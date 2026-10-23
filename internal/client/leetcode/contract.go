package leetcode

import (
	"context"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

type Client interface {
	GetCurrentDailyTask(ctx context.Context) (domain.Task, error)
}

// Profiles reads what LeetCode shows publicly about a user.
type Profiles interface {
	// GetUserCalendar returns domain.ErrUserNotFound for a name LeetCode does
	// not know.
	GetUserCalendar(ctx context.Context, username string) (domain.Calendar, error)
}
