package domain

type Task struct {
	Result Data `json:"data"`
}

type Data struct {
	ActiveDailyCodingChallengeQuestion DailyQuestion `json:"activeDailyCodingChallengeQuestion"`
}

type DailyQuestion struct {
	Date  string   `json:"date"`
	Link  string   `json:"link"`
	Quest Question `json:"question"`
}

type Question struct {
	AcRate     float64 `json:"acRate"`
	Difficulty string  `json:"difficulty"`
	Title      string  `json:"title"`
}
