package leetcode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// hangingServer accepts the request and never answers, like a connection
// left half-open after the host slept.
func hangingServer(t *testing.T) *httptest.Server {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	return srv
}

func fetchWithin(t *testing.T, fetch func() error, limit time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- fetch() }()

	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("fetch did not return within %s", limit)
		return nil
	}
}

func TestGetCurrentDailyTask_StopsWhenContextEnds(t *testing.T) {
	srv := hangingServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	fetch := func() error {
		_, err := New(srv.URL).GetCurrentDailyTask(ctx)
		return err
	}
	if err := fetchWithin(t, fetch, 5*time.Second); err == nil {
		t.Fatal("expected an error from a fetch that got no answer")
	}
}

func TestGetCurrentDailyTask_StopsAtClientTimeout(t *testing.T) {
	srv := hangingServer(t)

	c := New(srv.URL).(*client)
	c.http.Timeout = 50 * time.Millisecond

	fetch := func() error {
		_, err := c.GetCurrentDailyTask(context.Background())
		return err
	}
	if err := fetchWithin(t, fetch, 5*time.Second); err == nil {
		t.Fatal("expected an error from a fetch that got no answer")
	}
}

func TestGetCurrentDailyTask_ReadsTheDailyQuestion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"activeDailyCodingChallengeQuestion":{"date":"2026-10-11","link":"/problems/two-sum/","question":{"acRate":55.5,"difficulty":"Easy","title":"Two Sum"}}}}`))
	}))
	defer srv.Close()

	task, err := New(srv.URL).GetCurrentDailyTask(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	q := task.Result.ActiveDailyCodingChallengeQuestion
	if q.Quest.Title != "Two Sum" || q.Link != "/problems/two-sum/" || q.Quest.Difficulty != "Easy" {
		t.Fatalf("got %+v", q)
	}
}
