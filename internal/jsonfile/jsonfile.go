// Package jsonfile saves a value as a JSON file the safe way.
package jsonfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Write encodes v and writes it to path through a temporary file and a rename,
// so a crash never leaves a half-written file behind.
func Write(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir of %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file of %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	// Flush to disk before the rename, so a power loss cannot leave an empty file.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}

	return nil
}
