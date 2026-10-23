package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

const (
	DefaultSubscribersPath = "data/subscribers.json"
	DefaultLinksPath       = "data/leetcode-links.json"
)

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
}

type Storage struct {
	SubscribersPath string `toml:"subscribersPath"`
	LinksPath       string `toml:"linksPath"`
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

	if conf.Storage.LinksPath == "" {
		conf.Storage.LinksPath = DefaultLinksPath
	}

	return conf, nil
}
