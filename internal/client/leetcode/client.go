package leetcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/labstack/gommon/log"
)

type client struct {
	URL  string
	http *http.Client
}

func New(url string) Client {
	return &client{
		URL:  url,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *client) GetCurrentDailyTask(ctx context.Context) (domain.Task, error) {
	requestBody := struct {
		Query         string `json:"query"`
		OperationName string `json:"operationName"`
	}{
		Query: `query questionOfToday {
            activeDailyCodingChallengeQuestion {
                date
                link
                question {
                    acRate
                    difficulty
                    title
                }
            }
        }`,
		OperationName: "questionOfToday",
	}

	b, err := json.Marshal(requestBody)
	if err != nil {
		return domain.Task{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.URL+"/graphql", bytes.NewReader(b))
	if err != nil {
		return domain.Task{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.http.Do(req)
	if err != nil {
		return domain.Task{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Errorf("error while closing resp body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		msg, err := io.ReadAll(resp.Body)
		if err != nil {
			return domain.Task{}, fmt.Errorf("got non-OK resp (%s). Can't read the body: %v", resp.Status, err)
		}

		return domain.Task{}, fmt.Errorf("got non-OK resp (%s): %v", resp.Status, string(msg))
	}
	msg, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.Task{}, fmt.Errorf("got OK resp (%s). Can't read the body: %v", resp.Status, err)
	}

	var result domain.Task // не указатель
	if err := json.Unmarshal(msg, &result); err != nil {
		return domain.Task{}, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return result, nil
}
