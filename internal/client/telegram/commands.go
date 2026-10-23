package telegram

import (
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// splitCommand returns the command at the start of the message, like "show"
// or "settings@PingBot", and the text after it. The command is empty when the
// message does not start with one.
func splitCommand(msg *models.Message) (command, args string) {
	for _, e := range msg.Entities {
		if e.Type != models.MessageEntityTypeBotCommand || e.Offset != 0 {
			continue
		}
		if e.Length < 1 || e.Length > len(msg.Text) {
			return "", ""
		}

		return strings.TrimPrefix(msg.Text[:e.Length], "/"), strings.TrimSpace(msg.Text[e.Length:])
	}

	return "", ""
}

// isCommand matches a message that starts with /name. It also matches
// /name@<this bot's username>, in a group or a private chat; a command
// addressed to another bot, or to this bot while its username is unknown, does
// not match.
func (m *Menu) isCommand(name string) bot.MatchFunc {
	return func(update *models.Update) bool {
		if update.Message == nil {
			return false
		}

		command, _ := splitCommand(update.Message)
		command, addressee, addressed := strings.Cut(command, "@")
		if command != name {
			return false
		}

		return !addressed || (m.botUsername != "" && strings.EqualFold(addressee, m.botUsername))
	}
}
