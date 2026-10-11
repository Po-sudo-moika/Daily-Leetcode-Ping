package subscribers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Store keeps the chat ids of subscribers in a small JSON file, so that
// subscriptions survive a restart. It is safe for concurrent use.
type Store struct {
	path string

	mu    sync.Mutex
	chats map[int64]struct{}
}

type fileData struct {
	Chats []int64 `json:"chats"`
}

// Open loads the store from path. A missing file is an empty store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, chats: make(map[int64]struct{})}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read subscribers file: %w", err)
	}

	var data fileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse subscribers file %s: %w", path, err)
	}

	for _, chatID := range data.Chats {
		s.chats[chatID] = struct{}{}
	}

	return s, nil
}

// Add subscribes a chat. It reports whether the chat was new.
func (s *Store) Add(chatID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.chats[chatID]; ok {
		return false, nil
	}

	s.chats[chatID] = struct{}{}
	if err := s.save(); err != nil {
		delete(s.chats, chatID)
		return false, err
	}

	return true, nil
}

// Remove unsubscribes a chat. It reports whether the chat was subscribed.
func (s *Store) Remove(chatID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.chats[chatID]; !ok {
		return false, nil
	}

	delete(s.chats, chatID)
	if err := s.save(); err != nil {
		s.chats[chatID] = struct{}{}
		return false, err
	}

	return true, nil
}

func (s *Store) Has(chatID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.chats[chatID]
	return ok
}

// List returns the subscribed chat ids in ascending order.
func (s *Store) List() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sorted()
}

func (s *Store) sorted() []int64 {
	chats := make([]int64, 0, len(s.chats))
	for chatID := range s.chats {
		chats = append(chats, chatID)
	}
	slices.Sort(chats)

	return chats
}

// save writes to a temporary file and renames it, so a crash never leaves a
// half-written store behind. The caller holds s.mu.
func (s *Store) save() error {
	raw, err := json.MarshalIndent(fileData{Chats: s.sorted()}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode subscribers: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create subscribers dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp subscribers file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write subscribers file: %w", err)
	}
	// Flush to disk before the rename, so a power loss cannot leave an empty file.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync subscribers file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close subscribers file: %w", err)
	}

	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("replace subscribers file: %w", err)
	}

	return nil
}
