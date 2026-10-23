package telegram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
)

// TODO: simplify text modification
func getPostMessageSchema(question domain.DailyQuestion, lang subscribers.Language) string {
	acRateStr := strconv.FormatFloat(question.Quest.AcRate, 'f', 2, 64)

	escapedDate := question.Date
	escapedTitle := question.Quest.Title
	escapedDifficulty := question.Quest.Difficulty

	return escapeMarkdownV2(fmt.Sprintf(message{
		ru: `🎯 Daily LeetCode Challenge

📅 Дата: %s
📝 Задача: %s
🎚️ Сложность: %s
📊 AC Rate: %s%%

🔗 Ссылка: www.leetcode.com%s

Удачи в решении! 💻

#LeetCode`,
		en: `🎯 Daily LeetCode Challenge

📅 Date: %s
📝 Problem: %s
🎚️ Difficulty: %s
📊 AC Rate: %s%%

🔗 Link: www.leetcode.com%s

Good luck solving it! 💻

#LeetCode`,
	}.in(lang),
		escapedDate,
		escapedTitle,
		escapedDifficulty,
		acRateStr,
		question.Link))
}

func escapeMarkdownV2(text string) string {
	if text == "" {
		return text
	}

	specialChars := "_*[]()~`>#+-=|{}.!"

	var result strings.Builder
	result.Grow(len(text) + 10)

	for _, r := range text {
		if strings.ContainsRune(specialChars, r) {
			result.WriteRune('\\')
		}
		result.WriteRune(r)
	}

	return result.String()
}
