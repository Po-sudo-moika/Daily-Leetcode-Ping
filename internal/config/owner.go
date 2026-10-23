package config

import (
	"fmt"
	"strconv"
	"strings"
)

const OwnerEnvName = "TELEGRAM_OWNER_ID"

// LoadOwnerID returns the Telegram user id of the bot's owner from the
// TELEGRAM_OWNER_ID environment variable. When the variable is empty, it falls
// back to the .env file at dotEnvPath, like the bot token. 0 means no owner is
// set, which turns /chats off. A value that is not a number is an error; the
// message does not repeat the value.
func LoadOwnerID(dotEnvPath string, getenv func(string) string) (int64, error) {
	raw := strings.TrimSpace(getenv(OwnerEnvName))
	if raw == "" {
		values, err := readDotEnv(dotEnvPath)
		if err != nil {
			return 0, err
		}
		raw = strings.TrimSpace(values[OwnerEnvName])
	}

	if raw == "" {
		return 0, nil
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, the Telegram user id of the owner", OwnerEnvName)
	}

	return id, nil
}
