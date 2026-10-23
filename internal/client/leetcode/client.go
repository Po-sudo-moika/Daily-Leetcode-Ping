package leetcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/labstack/gommon/log"
)

type client struct {
	URL  string
	http *http.Client
}

func New(url string) Client {
	return newClient(url)
}

// NewProfiles returns the client for the public profile of a LeetCode user.
func NewProfiles(url string) Profiles {
	return newClient(url)
}

func newClient(url string) *client {
	return &client{
		URL:  url,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *client) GetCurrentDailyTask(ctx context.Context) (domain.Task, error) {
	var result domain.Task // не указатель
	err := l.query(ctx, `query questionOfToday {
            activeDailyCodingChallengeQuestion {
                date
                link
                question {
                    acRate
                    difficulty
                    title
                }
            }
        }`, "questionOfToday", nil, &result)
	if err != nil {
		return domain.Task{}, err
	}

	return result, nil
}

// GetUserCalendar reads what LeetCode shows publicly about a user's
// submissions. It needs no login. A name LeetCode does not know gives
// domain.ErrUserNotFound.
func (l *client) GetUserCalendar(ctx context.Context, username string) (domain.Calendar, error) {
	var result struct {
		Data struct {
			MatchedUser *struct {
				Username     string `json:"username"`
				UserCalendar struct {
					Streak             int    `json:"streak"`
					SubmissionCalendar string `json:"submissionCalendar"`
				} `json:"userCalendar"`
			} `json:"matchedUser"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	err := l.query(ctx, `query userCalendar($username: String!) {
            matchedUser(username: $username) {
                username
                userCalendar {
                    streak
                    submissionCalendar
                }
            }
        }`, "userCalendar", map[string]any{"username": username}, &result)
	if err != nil {
		return domain.Calendar{}, err
	}

	user := result.Data.MatchedUser
	if user == nil {
		// LeetCode answers 200 with an error for a name it does not know.
		for _, e := range result.Errors {
			if !strings.Contains(strings.ToLower(e.Message), "does not exist") {
				return domain.Calendar{}, fmt.Errorf("leetcode error: %s", e.Message)
			}
		}

		return domain.Calendar{}, domain.ErrUserNotFound
	}

	submissions := map[int64]int{}
	if raw := user.UserCalendar.SubmissionCalendar; raw != "" {
		var byDay map[string]int
		if err := json.Unmarshal([]byte(raw), &byDay); err != nil {
			return domain.Calendar{}, fmt.Errorf("failed to unmarshal submission calendar: %v", err)
		}
		for key, count := range byDay {
			day, err := strconv.ParseInt(key, 10, 64)
			if err != nil {
				return domain.Calendar{}, fmt.Errorf("bad day %q in submission calendar: %v", key, err)
			}
			submissions[day] = count
		}
	}

	return domain.Calendar{
		Username:    user.Username,
		BestStreak:  user.UserCalendar.Streak,
		Submissions: submissions,
	}, nil
}

// query posts a GraphQL query to LeetCode and reads the JSON answer into out.
func (l *client) query(ctx context.Context, query, operation string, variables map[string]any, out any) error {
	requestBody := struct {
		Query         string         `json:"query"`
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables,omitempty"`
	}{Query: query, OperationName: operation, Variables: variables}

	b, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.URL+"/graphql", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Errorf("error while closing resp body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		msg, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("got non-OK resp (%s). Can't read the body: %v", resp.Status, err)
		}

		return fmt.Errorf("got non-OK resp (%s): %v", resp.Status, string(msg))
	}
	msg, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("got OK resp (%s). Can't read the body: %v", resp.Status, err)
	}

	if err := json.Unmarshal(msg, out); err != nil {
		return fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return nil
}
