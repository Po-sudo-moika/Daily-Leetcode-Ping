package telegram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
)

// TODO: simplify text modification
func getPostMessageSchema(question domain.DailyQuestion) string {
	acRateStr := strconv.FormatFloat(question.Quest.AcRate, 'f', 2, 64)

	escapedDate := question.Date
	escapedTitle := question.Quest.Title
	escapedDifficulty := question.Quest.Difficulty

	return escapeMarkdownV2(fmt.Sprintf(`🎯 Daily LeetCode Challenge

📅 Дата: %s
📝 Задача: %s
🎚️ Сложность: %s
📊 AC Rate: %s%%

🔗 Ссылка: www.leetcode.com%s

Удачи в решении! 💻

#LeetCode`,
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
