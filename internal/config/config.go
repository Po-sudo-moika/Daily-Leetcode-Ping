package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

const DefaultSubscribersPath = "data/subscribers.json"

type Config struct {
	Service  Service
	Telegram Telegram
	Storage  Storage
}

type Service struct {
	Name       string `toml:"name"`
	Version    string `toml:"version"`
	Enviroment string `toml:"env"`
}

type Telegram struct {
	TelegramChats []string `toml:"telegramChats"`
	// OwnerID is the Telegram user id of the bot's owner, the only person
	// /chats answers. 0 means not set. The TELEGRAM_OWNER_ID environment
	// variable or .env entry wins over it (see LoadOwnerID).
	OwnerID int64 `toml:"ownerId"`
}

type Storage struct {
	SubscribersPath string `toml:"subscribersPath"`
}

// Load reads the config file and fills in defaults for optional fields.
func Load(path string) (Config, error) {
	var conf Config

	if _, err := toml.DecodeFile(path, &conf); err != nil {
		return Config{}, fmt.Errorf("decode config file %s: %w", path, err)
	}

	if conf.Storage.SubscribersPath == "" {
		conf.Storage.SubscribersPath = DefaultSubscribersPath
	}

	return conf, nil
}
