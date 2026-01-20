package leetcode

import (
	"context"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

type Client interface {
	GetCurrentDailyTask(ctx context.Context) (domain.Task, error)
}
