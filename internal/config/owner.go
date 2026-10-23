package config

import (
	"fmt"
	"strconv"
	"strings"
)

const OwnerEnvName = "TELEGRAM_OWNER_ID"

// LoadOwnerID returns the Telegram user id of the bot's owner. The
// TELEGRAM_OWNER_ID environment variable wins, then the same entry in the .env
// file at dotEnvPath, then telegram.ownerId from config.toml (configured). 0
// means no owner is set, which turns /chats off. A value that is not a number
// is an error; the message does not repeat the value.
func LoadOwnerID(dotEnvPath string, getenv func(string) string, configured int64) (int64, error) {
	raw := strings.TrimSpace(getenv(OwnerEnvName))
	if raw == "" {
		values, err := readDotEnv(dotEnvPath)
		if err != nil {
			return 0, err
		}
		raw = strings.TrimSpace(values[OwnerEnvName])
	}

	if raw == "" {
		return configured, nil
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, the Telegram user id of the owner", OwnerEnvName)
	}

	return id, nil
}
