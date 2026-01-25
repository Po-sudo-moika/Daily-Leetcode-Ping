package config

type Config struct {
	Service  Service
	Telegram Telegram
}

type Service struct {
	Name       string `toml:"name"`
	Version    string `toml:"version"`
	Enviroment string `toml:"env"`
}

type Telegram struct {
	BotCode       string   `toml:"botCode"`
	TelegramChats []string `toml:"telegramChats"`
}
