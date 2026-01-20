package main

import (
	"context"
	"fmt"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/leetcode"
)

func main() {
	leetcodeClient := leetcode.New("https://leetcode.com")

	ctx := context.Background()

	res, err := leetcodeClient.GetCurrentDailyTask(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(res)
}
