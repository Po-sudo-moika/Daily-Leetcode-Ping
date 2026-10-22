package subscribers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
)

const (
	// DefaultHour is the Moscow hour of the daily post for a new subscriber,
	// and for every subscriber saved before the hour could be chosen.
	DefaultHour = 11

	// MinHour and MaxHour bound the Moscow hour a subscriber can choose.
	// LeetCode changes the daily problem at 00:00 UTC, which is 03:00 in
	// Moscow (UTC+3, no daylight saving). Starting at 04:00 leaves a full hour
	// of margin, so a post never carries the previous day's problem.
	MinHour = 4
	MaxHour = 23
)

// ErrInvalidHour means the hour is outside MinHour..MaxHour.
var ErrInvalidHour = errors.New("hour out of range")

// ErrNotSubscribed means the chat has no subscription.
var ErrNotSubscribed = errors.New("chat is not subscribed")

// Store keeps the subscribers, their delivery hour, the chats that chose to
// unsubscribe and the day each chat last got the post in a small JSON file, so
// that all of it survives a restart. It is safe for concurrent use.
//
// A day is a Moscow date in time.DateOnly form, such as "2026-10-11". A chat
// in the delivery records is a subscriber's id or a chat from config.toml.
type Store struct {
	path string

	mu sync.Mutex
	// chats maps a subscribed chat id to its Moscow delivery hour.
	chats map[int64]int
	// optedOut holds the chats that pressed Unsubscribe. Only a deliberate
	// subscribe (Add) takes a chat out of it; meeting the chat again (Meet)
	// does not.
	optedOut map[int64]bool
	// delivered maps a chat to the last day it got the post.
	delivered map[string]string
}

// fileData is the file format. Chats is the original format and stays a plain
// list of ids, so a file written before delivery hours existed loads as is and
// an older build can still read a newer file. Hours lists only the chats that
// chose something other than DefaultHour, keyed by chat id. OptedOut lists the
// chats that pressed Unsubscribe. Delivered maps a chat to the last day it got
// the post.
type fileData struct {
	Chats     []int64           `json:"chats"`
	Hours     map[string]int    `json:"hours,omitempty"`
	OptedOut  []int64           `json:"optedOut,omitempty"`
	Delivered map[string]string `json:"delivered,omitempty"`
}

// Open loads the store from path. A missing file is an empty store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, chats: make(map[int64]int), optedOut: make(map[int64]bool), delivered: make(map[string]string)}

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
		s.chats[chatID] = DefaultHour
	}

	for key, hour := range data.Hours {
		chatID, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse subscribers file %s: bad chat id %q in hours", path, key)
		}
		// An hour for a chat that is not subscribed, or one outside the
		// allowed range, is ignored: the subscriber keeps DefaultHour.
		if _, ok := s.chats[chatID]; ok && validHour(hour) {
			s.chats[chatID] = hour
		}
	}

	for _, chatID := range data.OptedOut {
		// A chat that is subscribed in the same file is a subscriber.
		if _, ok := s.chats[chatID]; !ok {
			s.optedOut[chatID] = true
		}
	}

	for chat, day := range data.Delivered {
		s.delivered[chat] = day
	}

	return s, nil
}

func validHour(hour int) bool {
	return hour >= MinHour && hour <= MaxHour
}

// Add subscribes a chat, because someone asked for it: the Subscribe button,
// or the bot being added to a group. It takes the chat out of the unsubscribed
// ones. It reports whether the chat was new.
func (s *Store) Add(chatID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.chats[chatID]; ok {
		return false, nil
	}

	return true, s.subscribe(chatID)
}

// Meet subscribes a chat the bot meets for the first time: it has no
// subscription and never pressed Unsubscribe. It reports whether it subscribed
// the chat. A subscriber and a chat that unsubscribed are left as they are.
func (s *Store) Meet(chatID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.chats[chatID]; ok || s.optedOut[chatID] {
		return false, nil
	}

	if err := s.subscribe(chatID); err != nil {
		return false, err
	}

	return true, nil
}

// subscribe adds the chat at DefaultHour and forgets that it unsubscribed. A
// failed save leaves the store as it was. The caller holds s.mu.
func (s *Store) subscribe(chatID int64) error {
	wasOptedOut := s.optedOut[chatID]
	s.chats[chatID] = DefaultHour
	delete(s.optedOut, chatID)

	if err := s.save(); err != nil {
		delete(s.chats, chatID)
		if wasOptedOut {
			s.optedOut[chatID] = true
		}
		return err
	}

	return nil
}

// Remove unsubscribes a chat because someone chose to, and remembers the
// choice: Meet does not subscribe the chat again, also after a restart. It
// reports whether the chat was subscribed.
func (s *Store) Remove(chatID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hour, wasSubscribed := s.chats[chatID]
	if !wasSubscribed && s.optedOut[chatID] {
		return false, nil
	}

	delete(s.chats, chatID)
	s.optedOut[chatID] = true
	if err := s.save(); err != nil {
		delete(s.optedOut, chatID)
		if wasSubscribed {
			s.chats[chatID] = hour
		}
		return false, err
	}

	return wasSubscribed, nil
}

// Drop unsubscribes a chat that cannot get the post any more: the bot was
// blocked or removed, or the chat is gone. It does not count as a choice: a
// chat that comes back is met again and subscribed, unless it had pressed
// Unsubscribe before. It reports whether the chat was subscribed.
func (s *Store) Drop(chatID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hour, ok := s.chats[chatID]
	if !ok {
		return false, nil
	}

	delete(s.chats, chatID)
	if err := s.save(); err != nil {
		s.chats[chatID] = hour
		return false, err
	}

	return true, nil
}

// Hour returns the Moscow delivery hour of a chat. The second result is false
// when the chat is not subscribed.
func (s *Store) Hour(chatID int64) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hour, ok := s.chats[chatID]

	return hour, ok
}

// SetHour changes the Moscow delivery hour of a subscribed chat. It returns
// ErrInvalidHour for an hour outside MinHour..MaxHour and ErrNotSubscribed
// for a chat without a subscription.
func (s *Store) SetHour(chatID int64, hour int) error {
	if !validHour(hour) {
		return fmt.Errorf("%w: %d (allowed %d..%d)", ErrInvalidHour, hour, MinHour, MaxHour)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old, ok := s.chats[chatID]
	if !ok {
		return ErrNotSubscribed
	}

	s.chats[chatID] = hour
	if err := s.save(); err != nil {
		s.chats[chatID] = old
		return err
	}

	return nil
}

// Due returns the subscribed chats whose delivery hour is at or before hour
// and that have not got the post on day, in ascending order.
func (s *Store) Due(hour int, day string) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	var chats []int64
	for chatID, h := range s.chats {
		if h <= hour && s.delivered[strconv.FormatInt(chatID, 10)] != day {
			chats = append(chats, chatID)
		}
	}
	slices.Sort(chats)

	return chats
}

// Delivered reports whether chat got the post on day.
func (s *Store) Delivered(chat, day string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.delivered[chat] == day
}

// MarkDelivered records that chat got the post on day. Records of other days
// are dropped: for deciding who is due on day they mean the same as none, and
// this keeps chats that left from piling up in the file. A failed save keeps
// the record in memory, because the post did go out and must not go again.
func (s *Store) MarkDelivered(chat, day string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for other, d := range s.delivered {
		if d != day {
			delete(s.delivered, other)
		}
	}
	s.delivered[chat] = day

	return s.save()
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
	data := fileData{Chats: s.sorted(), Delivered: s.delivered}
	for chatID := range s.optedOut {
		data.OptedOut = append(data.OptedOut, chatID)
	}
	slices.Sort(data.OptedOut)
	for chatID, hour := range s.chats {
		if hour == DefaultHour {
			continue
		}
		if data.Hours == nil {
			data.Hours = make(map[string]int)
		}
		data.Hours[strconv.FormatInt(chatID, 10)] = hour
	}

	raw, err := json.MarshalIndent(data, "", "  ")
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
