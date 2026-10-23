package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOwnerID(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no.env")

	for name, tc := range map[string]struct {
		env        map[string]string
		dotEnv     string
		configured int64
		want       int64
	}{
		"nothing set":                {nil, "", 0, 0},
		"config value":               {nil, "", 42, 42},
		"environment wins":           {map[string]string{OwnerEnvName: "7"}, OwnerEnvName + "=8\n", 42, 7},
		".env wins over config":      {nil, OwnerEnvName + "=8\n", 42, 8},
		"blank environment is unset": {map[string]string{OwnerEnvName: "  "}, "", 42, 42},
	} {
		t.Run(name, func(t *testing.T) {
			path := missing
			if tc.dotEnv != "" {
				path = writeDotEnv(t, tc.dotEnv)
			}

			got, err := LoadOwnerID(path, envOf(tc.env), tc.configured)
			if err != nil || got != tc.want {
				t.Fatalf("got %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

func TestLoadOwnerID_NotANumberIsAnErrorThatDoesNotRepeatTheValue(t *testing.T) {
	_, err := LoadOwnerID(filepath.Join(t.TempDir(), "no.env"), envOf(map[string]string{OwnerEnvName: "12ab"}), 0)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "12ab") {
		t.Fatalf("the error repeats the value: %v", err)
	}
}

func TestLoad_OwnerID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[telegram]\nownerId = 1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	conf, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if conf.Telegram.OwnerID != 1234 {
		t.Fatalf("owner id %d, want 1234", conf.Telegram.OwnerID)
	}
}
