package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const TokenEnvName = "TELEGRAM_BOT_TOKEN"

var ErrTokenMissing = fmt.Errorf(
	"%s is not set: export it or put %s=... into a .env file in the working directory",
	TokenEnvName, TokenEnvName,
)

// LoadToken returns the bot token from the TELEGRAM_BOT_TOKEN environment
// variable. When the variable is empty, it falls back to the .env file at
// dotEnvPath. A missing .env file is fine; a missing token is not.
func LoadToken(dotEnvPath string, getenv func(string) string) (string, error) {
	if token := strings.TrimSpace(getenv(TokenEnvName)); token != "" {
		return token, nil
	}

	values, err := readDotEnv(dotEnvPath)
	if err != nil {
		return "", err
	}

	if token := strings.TrimSpace(values[TokenEnvName]); token != "" {
		return token, nil
	}

	return "", ErrTokenMissing
}

// readDotEnv parses simple KEY=VALUE lines. It skips blank lines and
// comments, accepts an optional "export " prefix and strips one pair of
// surrounding quotes. A missing file gives an empty result.
func readDotEnv(path string) (map[string]string, error) {
	values := make(map[string]string)

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return values, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		values[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return values, nil
}

func unquote(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if first == last && (first == '"' || first == '\'') {
			return value[1 : len(value)-1]
		}
	}

	return value
}
