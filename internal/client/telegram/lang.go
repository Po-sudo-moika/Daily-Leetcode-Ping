package telegram

import "github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"

// message is one text in each language a chat can read the bot in.
type message struct {
	ru, en string
}

// in returns the text in lang; Russian for anything but English.
func (m message) in(lang subscribers.Language) string {
	if lang == subscribers.English {
		return m.en
	}

	return m.ru
}
