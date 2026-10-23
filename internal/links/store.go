// Package links keeps which LeetCode username each Telegram user linked.
package links

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/jsonfile"
	"github.com/labstack/gommon/log"
)

// MaxLinks is a high safety cap on the stored links, one per Telegram user. A
// real bot stays far below it. Past it Set refuses a new user with ErrFull, so a
// flood of new users cannot grow the file without bound. A user who already
// linked can still change their link.
const MaxLinks = 50000

// ErrFull means the store holds MaxLinks links and refuses a new user.
var ErrFull = errors.New("links store is full")

// Link is one person's link. Only the public LeetCode username is kept, never
// a login token or cookie.
type Link struct {
	UserID int64 `json:"userId"`
	// Login is the Telegram @login as last seen, without the "@"; empty when
	// the person has none.
	Login    string `json:"login,omitempty"`
	LeetCode string `json:"leetcode"`
}

// Store keeps the links in a small JSON file, so they survive a restart. It is
// safe for concurrent use.
type Store struct {
	path string

	mu    sync.Mutex
	users map[int64]Link
}

type fileData struct {
	Links []Link `json:"links"`
}

// Open loads the store from path. A missing file is an empty store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, users: make(map[int64]Link)}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read links file: %w", err)
	}

	var data fileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse links file %s: %w", path, err)
	}
	for _, link := range data.Links {
		s.users[link.UserID] = link
	}

	return s, nil
}

// Set links the user to a LeetCode username and records their @login. It
// replaces an earlier link of the same user. A failed save leaves the store as
// it was.
func (s *Store) Set(userID int64, login, leetcode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, linked := s.users[userID]; !linked && len(s.users) >= MaxLinks {
		log.Warnf("links store is full (%d links): refusing a new link", MaxLinks)
		return ErrFull
	}

	return s.change(func() {
		s.users[userID] = Link{UserID: userID, Login: login, LeetCode: leetcode}
		s.forgetLoginOfOthers(userID, login)
	})
}

// Touch refreshes the @login of a user who linked, so a changed Telegram name
// is picked up. It does nothing for a user who did not link, or when the login
// is the same.
func (s *Store) Touch(userID int64, login string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.users[userID]
	if !ok || link.Login == login {
		return nil
	}

	return s.change(func() {
		link.Login = login
		s.users[userID] = link
		s.forgetLoginOfOthers(userID, login)
	})
}

// forgetLoginOfOthers takes login away from every other user: a Telegram @login
// belongs to one person at a time, and the last one seen wins. The caller
// holds s.mu.
func (s *Store) forgetLoginOfOthers(userID int64, login string) {
	if login == "" {
		return
	}

	for id, link := range s.users {
		if id != userID && strings.EqualFold(link.Login, login) {
			link.Login = ""
			s.users[id] = link
		}
	}
}

// Get returns the link of a user.
func (s *Store) Get(userID int64) (Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.users[userID]

	return link, ok
}

// ByLogin finds the link of the user with this Telegram @login. The case and a
// leading "@" do not matter.
func (s *Store) ByLogin(login string) (Link, bool) {
	login = strings.TrimPrefix(login, "@")
	if login == "" {
		return Link{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, link := range s.users {
		if strings.EqualFold(link.Login, login) {
			return link, true
		}
	}

	return Link{}, false
}

// Remove deletes the link of a user. It reports whether there was one.
func (s *Store) Remove(userID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return false, nil
	}

	if err := s.change(func() { delete(s.users, userID) }); err != nil {
		return false, err
	}

	return true, nil
}

// change applies the edit and saves. A failed save puts the store back as it
// was. The caller holds s.mu.
func (s *Store) change(edit func()) error {
	before := make(map[int64]Link, len(s.users))
	for id, link := range s.users {
		before[id] = link
	}

	edit()
	if err := s.save(); err != nil {
		s.users = before
		return err
	}

	return nil
}

func (s *Store) save() error {
	data := fileData{Links: make([]Link, 0, len(s.users))}
	for _, link := range s.users {
		data.Links = append(data.Links, link)
	}
	slices.SortFunc(data.Links, func(a, b Link) int { return cmp.Compare(a.UserID, b.UserID) })

	return jsonfile.Write(s.path, data)
}
