package leetcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/labstack/gommon/log"
)

type client struct {
	URL string
}

func New(url string) Client {
	return &client{
		URL: url,
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

	resp, err := http.Post(l.URL+"/graphql", "application/json", bytes.NewReader(b))
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
