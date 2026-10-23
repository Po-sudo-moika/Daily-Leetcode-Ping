package domain

import (
	"errors"
	"time"
)

// ErrUserNotFound means LeetCode does not know the username.
var ErrUserNotFound = errors.New("leetcode user not found")

const secondsPerDay = 24 * 60 * 60

// Calendar is what LeetCode shows publicly about one user's submissions.
type Calendar struct {
	// Username is the name as LeetCode spells it.
	Username string
	// BestStreak is the longest run of days with a submission in the
	// calendar's period, as LeetCode counts it.
	BestStreak int
	// Submissions maps the Unix time of the start of a UTC day to the number
	// of submissions made on it.
	Submissions map[int64]int
}

// CurrentStreak is the number of UTC days in a row, up to today, with at least
// one submission. A run that ended yesterday still counts, because today is
// not over; a run that ended earlier is 0.
func (c Calendar) CurrentStreak(now time.Time) int {
	day := now.UTC().Unix()
	day -= day % secondsPerDay

	if c.Submissions[day] == 0 {
		day -= secondsPerDay
	}

	streak := 0
	for c.Submissions[day] > 0 {
		streak++
		day -= secondsPerDay
	}

	return streak
}
