package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := "[telegram]\ntelegramChats = [\"1\", \"@channel\"]\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	conf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(conf.Telegram.TelegramChats) != 2 || conf.Telegram.TelegramChats[1] != "@channel" {
		t.Fatalf("unexpected chats: %v", conf.Telegram.TelegramChats)
	}
	if conf.Storage.SubscribersPath != DefaultSubscribersPath {
		t.Fatalf("got path %q, want default %q", conf.Storage.SubscribersPath, DefaultSubscribersPath)
	}
}

func TestLoad_CustomSubscribersPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := "[storage]\nsubscribersPath = \"/var/lib/ping/subs.json\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	conf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conf.Storage.SubscribersPath != "/var/lib/ping/subs.json" {
		t.Fatalf("got path %q", conf.Storage.SubscribersPath)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("want an error for a missing file")
	}
}
