package leetcode

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

const (
	// cacheTTL is how long an answer about a username is kept. Within it, asking
	// again for the same person makes no call to LeetCode.
	cacheTTL = 5 * time.Minute
	// maxCacheEntries bounds the memory of the cache.
	maxCacheEntries = 1000
	// maxInFlight is how many calls to LeetCode run at the same time.
	maxInFlight = 3
	// callInterval is the least time between the starts of two calls: about one
	// call a second.
	callInterval = time.Second
	// maxWait is how long a request may wait for its turn. A request that would
	// wait longer gets ErrBusy, so waiting requests never pile up without bound.
	maxWait = 3 * time.Second
)

// ErrBusy means the guard let no more calls through for now. The person can try
// again a little later.
var ErrBusy = errors.New("too many requests to LeetCode, try again later")

// usernamePattern is what a LeetCode username looks like.
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,40}$`)

// ValidUsername reports whether name has the form of a LeetCode username. It
// also keeps odd text out of the questions sent to LeetCode.
func ValidUsername(name string) bool {
	return usernamePattern.MatchString(name)
}

// Guarded protects LeetCode, and this server's address, from too many calls
// about users. It checks the form of the username before any call, keeps the
// answers for a few minutes, and lets only a few calls run at once, about one a
// second. It is safe for concurrent use.
type Guarded struct {
	next Profiles
	now  func() time.Time
	// slots has one place for each call that may run at the same time.
	slots chan struct{}

	mu       sync.Mutex
	cache    map[string]cached
	nextCall time.Time
}

// cached is an answer kept for a username: a calendar, or domain.ErrUserNotFound.
type cached struct {
	calendar domain.Calendar
	err      error
	until    time.Time
}

// NewGuarded returns next behind the guard.
func NewGuarded(next Profiles) *Guarded {
	return &Guarded{
		next:  next,
		now:   time.Now,
		slots: make(chan struct{}, maxInFlight),
		cache: make(map[string]cached),
	}
}

// GetUserCalendar answers from the cache when it can. A username of a wrong
// form is an unknown user, with no call. When the guard has no turn to give in
// time, it returns ErrBusy. Only a calendar and an unknown user are kept: any
// other error is asked again next time. The calendar is shared, so the caller
// must not change it.
func (g *Guarded) GetUserCalendar(ctx context.Context, username string) (domain.Calendar, error) {
	if !ValidUsername(username) {
		return domain.Calendar{}, domain.ErrUserNotFound
	}

	key := strings.ToLower(username)
	if c, ok := g.lookup(key); ok {
		return c.calendar, c.err
	}

	if err := g.acquire(ctx); err != nil {
		return domain.Calendar{}, err
	}
	defer func() { <-g.slots }()

	calendar, err := g.next.GetUserCalendar(ctx, username)
	if err == nil || errors.Is(err, domain.ErrUserNotFound) {
		g.keep(key, cached{calendar: calendar, err: err})
	}

	return calendar, err
}

func (g *Guarded) lookup(key string) (cached, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	c, ok := g.cache[key]

	return c, ok && g.now().Before(c.until)
}

func (g *Guarded) keep(key string, c cached) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	if _, known := g.cache[key]; !known && len(g.cache) >= maxCacheEntries {
		for k, old := range g.cache {
			if !now.Before(old.until) {
				delete(g.cache, k)
			}
		}
		for k := range g.cache {
			if len(g.cache) < maxCacheEntries {
				break
			}
			delete(g.cache, k)
		}
	}

	c.until = now.Add(cacheTTL)
	g.cache[key] = c
}

// acquire waits for a place among the running calls and for the turn in the
// one-call-a-second line, and returns ErrBusy when both together take longer
// than maxWait. The caller releases the place with <-g.slots.
func (g *Guarded) acquire(ctx context.Context) error {
	start := time.Now()
	timeout := time.NewTimer(maxWait)
	defer timeout.Stop()

	select {
	case g.slots <- struct{}{}:
	case <-timeout.C:
		return ErrBusy
	case <-ctx.Done():
		return ctx.Err()
	}

	wait, ok := g.reserve(maxWait - time.Since(start))
	if !ok {
		<-g.slots
		return ErrBusy
	}
	if wait <= 0 {
		return nil
	}

	turn := time.NewTimer(wait)
	defer turn.Stop()

	select {
	case <-turn.C:
		return nil
	case <-ctx.Done():
		<-g.slots
		return ctx.Err()
	}
}

// reserve books the next turn and returns how long to wait for it. It reports
// false, booking nothing, when the turn is more than budget away.
func (g *Guarded) reserve(budget time.Duration) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	at := g.nextCall
	if at.Before(now) {
		at = now
	}
	if at.Sub(now) > budget {
		return 0, false
	}

	g.nextCall = at.Add(callInterval)

	return at.Sub(now), true
}
