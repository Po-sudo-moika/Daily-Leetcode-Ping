package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOwnerID(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no.env")

	for name, tc := range map[string]struct {
		env    map[string]string
		dotEnv string
		want   int64
	}{
		"nothing set":                {nil, "", 0},
		"environment":                {map[string]string{OwnerEnvName: " 7 "}, "", 7},
		".env":                       {nil, OwnerEnvName + "=8\n", 8},
		"environment wins over .env": {map[string]string{OwnerEnvName: "7"}, OwnerEnvName + "=8\n", 7},
		"blank environment is unset": {map[string]string{OwnerEnvName: "  "}, OwnerEnvName + "=8\n", 8},
	} {
		t.Run(name, func(t *testing.T) {
			path := missing
			if tc.dotEnv != "" {
				path = writeDotEnv(t, tc.dotEnv)
			}

			got, err := LoadOwnerID(path, envOf(tc.env))
			if err != nil || got != tc.want {
				t.Fatalf("got %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

func TestLoadOwnerID_NotANumberIsAnErrorThatDoesNotRepeatTheValue(t *testing.T) {
	_, err := LoadOwnerID(filepath.Join(t.TempDir(), "no.env"), envOf(map[string]string{OwnerEnvName: "12ab"}))
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "12ab") {
		t.Fatalf("the error repeats the value: %v", err)
	}
}
