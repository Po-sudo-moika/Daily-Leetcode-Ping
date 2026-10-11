package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeToken = "123456:fake-token-for-tests"

func envOf(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func writeDotEnv(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestLoadToken_FromEnvironment(t *testing.T) {
	got, err := LoadToken(filepath.Join(t.TempDir(), ".env"), envOf(map[string]string{TokenEnvName: " " + fakeToken + " "}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeToken {
		t.Fatalf("got %q, want %q", got, fakeToken)
	}
}

func TestLoadToken_EnvironmentBeatsDotEnv(t *testing.T) {
	path := writeDotEnv(t, TokenEnvName+"=from-file\n")

	got, err := LoadToken(path, envOf(map[string]string{TokenEnvName: fakeToken}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeToken {
		t.Fatalf("got %q, want %q", got, fakeToken)
	}
}

func TestLoadToken_FromDotEnv(t *testing.T) {
	tests := map[string]string{
		"plain":         TokenEnvName + "=" + fakeToken + "\n",
		"no newline":    TokenEnvName + "=" + fakeToken,
		"double quotes": TokenEnvName + "=\"" + fakeToken + "\"\n",
		"single quotes": TokenEnvName + "='" + fakeToken + "'\n",
		"export prefix": "export " + TokenEnvName + "=" + fakeToken + "\n",
		"comments":      "# bot\n\nOTHER=1\n" + TokenEnvName + " = " + fakeToken + "\n",
		"crlf":          TokenEnvName + "=" + fakeToken + "\r\n",
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := LoadToken(writeDotEnv(t, content), envOf(nil))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != fakeToken {
				t.Fatalf("got %q, want %q", got, fakeToken)
			}
		})
	}
}

func TestLoadToken_Missing(t *testing.T) {
	tests := map[string]string{
		"no file":        "",
		"empty file":     "\n",
		"other keys":     "OTHER=1\n",
		"empty value":    TokenEnvName + "=\n",
		"commented out":  "# " + TokenEnvName + "=" + fakeToken + "\n",
		"quoted nothing": TokenEnvName + "=\"\"\n",
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if name != "no file" {
				path = writeDotEnv(t, content)
			}

			got, err := LoadToken(path, envOf(nil))
			if !errors.Is(err, ErrTokenMissing) {
				t.Fatalf("got err %v, want ErrTokenMissing", err)
			}
			if got != "" {
				t.Fatalf("got token %q, want empty", got)
			}
			if !strings.Contains(err.Error(), TokenEnvName) {
				t.Fatalf("message %q does not name %s", err, TokenEnvName)
			}
		})
	}
}

func TestLoadToken_UnreadableDotEnv(t *testing.T) {
	// A directory where the file should be: exists, but cannot be read as a file.
	_, err := LoadToken(t.TempDir(), envOf(nil))
	if err == nil || errors.Is(err, ErrTokenMissing) {
		t.Fatalf("got err %v, want a read error", err)
	}
}
