package subscribers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"
)

// A delivery time is a Moscow time of day, counted in minutes since midnight:
// 09:30 is 570. Use At to build one and Format to print one.
const (
	// DefaultHour is the Moscow hour of the daily post for a new subscriber,
	// and for every subscriber saved before the time could be chosen.
	DefaultHour = 11

	// MinHour and MaxHour bound the Moscow hour a subscriber can choose.
	// LeetCode changes the daily problem at 00:00 UTC, which is 03:00 in
	// Moscow (UTC+3, no daylight saving). Starting at 04:00 leaves a full hour
	// of margin, so a post never carries the previous day's problem.
	MinHour = 4
	MaxHour = 23

	// MinuteStep is the step of the minutes a subscriber can choose.
	MinuteStep = 5

	// DefaultTime, MinTime and MaxTime are DefaultHour, MinHour and MaxHour
	// as delivery times; the last one is 23:55.
	DefaultTime = DefaultHour * 60
	MinTime     = MinHour * 60
	MaxTime     = MaxHour*60 + 60 - MinuteStep

	// deliveredKeepDays is how long the day a chat last got the post is kept.
	deliveredKeepDays = 90
)

// ErrInvalidTime means the time is outside MinTime..MaxTime or is not a
// multiple of MinuteStep minutes.
var ErrInvalidTime = errors.New("time out of range")

// At returns the delivery time of hour:minute.
func At(hour, minute int) int {
	return hour*60 + minute
}

// Format prints a delivery time as "09:30".
func Format(t int) string {
	return fmt.Sprintf("%02d:%02d", t/60, t%60)
}

// Now returns the delivery time of t, in the time zone of t: the moment from
// which a chat with a delivery time at or before it is due.
func Now(t time.Time) int {
	return At(t.Hour(), t.Minute())
}

// Language is the language a chat reads the bot in.
type Language string

const (
	// Russian is the language of every chat until it switches.
	Russian Language = "ru"
	English Language = "en"
)

// Other returns the language a chat switches to: English for Russian and
// Russian for anything else.
func (l Language) Other() Language {
	if l == Russian {
		return English
	}

	return Russian
}

// ErrNotSubscribed means the chat has no subscription.
var ErrNotSubscribed = errors.New("chat is not subscribed")

// Store keeps the subscribers, their delivery time, the chats that chose to
// unsubscribe and the day each chat last got the post in a small JSON file, so
// that all of it survives a restart. It is safe for concurrent use.
//
// A day is a Moscow date in time.DateOnly form, such as "2026-10-11". A chat
// in the delivery records is a subscriber's id or a chat from config.toml.
type Store struct {
	path string

	mu sync.Mutex
	// chats maps a subscribed chat id to its Moscow delivery time.
	chats map[int64]int
	// optedOut holds the chats that pressed Unsubscribe. Only a deliberate
	// subscribe (Add) takes a chat out of it; meeting the chat again (Meet)
	// does not.
	optedOut map[int64]bool
	// delivered maps a chat to the last day it got the post.
	delivered map[string]string
	// languages holds the chats whose language is not Russian. It is kept
	// whether or not the chat is subscribed.
	languages map[int64]Language
	// greeted holds the chats that were sent the hello of the first /start.
	greeted map[int64]bool
}

// fileData is the file format. Chats is the original format and stays a plain
// list of ids, so a file written before delivery hours existed loads as is and
// an older build can still read a newer file. Hours lists only the chats whose
// time is not DefaultHour:00, keyed by chat id, and Minutes lists the minutes
// of those that are not on the hour. An older build reads the hour and drops
// the minutes. OptedOut lists the chats that pressed Unsubscribe. Delivered
// maps a chat to the last day it got the post. Languages maps a chat id to its
// language when it is not Russian, and Greeted lists the chats that got the
// hello of their first /start; an older file has neither, so every chat loads
// in Russian and not yet greeted.
type fileData struct {
	Chats     []int64           `json:"chats"`
	Hours     map[string]int    `json:"hours,omitempty"`
	Minutes   map[string]int    `json:"minutes,omitempty"`
	OptedOut  []int64           `json:"optedOut,omitempty"`
	Delivered map[string]string `json:"delivered,omitempty"`
	Languages map[string]string `json:"languages,omitempty"`
	Greeted   []int64           `json:"greeted,omitempty"`
}

// Open loads the store from path. A missing file is an empty store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, chats: make(map[int64]int), optedOut: make(map[int64]bool), delivered: make(map[string]string),
		languages: make(map[int64]Language), greeted: make(map[int64]bool)}

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
		s.chats[chatID] = DefaultTime
	}

	for key, hour := range data.Hours {
		chatID, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse subscribers file %s: bad chat id %q in hours", path, key)
		}
		// An hour for a chat that is not subscribed, or one outside the
		// allowed range, is ignored: the subscriber keeps DefaultHour.
		if _, ok := s.chats[chatID]; ok && validHour(hour) {
			s.chats[chatID] = At(hour, 0)
		}
	}

	for key, minute := range data.Minutes {
		chatID, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse subscribers file %s: bad chat id %q in minutes", path, key)
		}
		// A minute for a chat without a valid hour, or one that is not a
		// step of MinuteStep, is ignored: the chat keeps its hour at :00.
		if t, ok := s.chats[chatID]; ok && validTime(t+minute) {
			s.chats[chatID] = t + minute
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

	for key, lang := range data.Languages {
		chatID, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse subscribers file %s: bad chat id %q in languages", path, key)
		}
		// A language the bot does not know is ignored: the chat stays Russian.
		if Language(lang) == English {
			s.languages[chatID] = English
		}
	}

	for _, chatID := range data.Greeted {
		s.greeted[chatID] = true
	}

	return s, nil
}

func validHour(hour int) bool {
	return hour >= MinHour && hour <= MaxHour
}

func validTime(t int) bool {
	return t >= MinTime && t <= MaxTime && t%MinuteStep == 0
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

// subscribe adds the chat at DefaultTime and forgets that it unsubscribed. A
// failed save leaves the store as it was. The caller holds s.mu.
func (s *Store) subscribe(chatID int64) error {
	wasOptedOut := s.optedOut[chatID]
	s.chats[chatID] = DefaultTime
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

// Migrate moves a chat to its new id when Telegram turns a group into a
// supergroup: its subscription and time, or its Unsubscribe, its language and
// greeting, and the day it last got the post. The old id is forgotten. It reports whether there was
// anything to move. A failed save leaves the store as it was.
func (s *Store) Migrate(from, to int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hour, subscribed := s.chats[from]
	lang, hasLang := s.languages[from]
	greeted := s.greeted[from]
	if !subscribed && !s.optedOut[from] && !hasLang && !greeted {
		return false, nil
	}

	chats, optedOut, delivered := maps.Clone(s.chats), maps.Clone(s.optedOut), maps.Clone(s.delivered)
	languages, greetedChats := maps.Clone(s.languages), maps.Clone(s.greeted)

	delete(s.chats, to)
	delete(s.optedOut, to)
	if subscribed {
		s.chats[to] = hour
	} else if s.optedOut[from] {
		s.optedOut[to] = true
	}
	delete(s.chats, from)
	delete(s.optedOut, from)

	delete(s.languages, to)
	delete(s.greeted, to)
	if hasLang {
		s.languages[to] = lang
	}
	if greeted {
		s.greeted[to] = true
	}
	delete(s.languages, from)
	delete(s.greeted, from)

	fromKey := strconv.FormatInt(from, 10)
	if day, ok := s.delivered[fromKey]; ok {
		s.delivered[strconv.FormatInt(to, 10)] = day
		delete(s.delivered, fromKey)
	}

	if err := s.save(); err != nil {
		s.chats, s.optedOut, s.delivered = chats, optedOut, delivered
		s.languages, s.greeted = languages, greetedChats
		return false, err
	}

	return true, nil
}

// Time returns the Moscow delivery time of a chat. The second result is false
// when the chat is not subscribed.
func (s *Store) Time(chatID int64) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.chats[chatID]

	return t, ok
}

// SetTime changes the Moscow delivery time of a subscribed chat. It returns
// ErrInvalidTime for a time outside MinTime..MaxTime or off the MinuteStep
// grid, and ErrNotSubscribed for a chat without a subscription.
func (s *Store) SetTime(chatID int64, t int) error {
	if !validTime(t) {
		return fmt.Errorf("%w: %d (allowed %s..%s every %d minutes)", ErrInvalidTime, t, Format(MinTime), Format(MaxTime), MinuteStep)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old, ok := s.chats[chatID]
	if !ok {
		return ErrNotSubscribed
	}

	s.chats[chatID] = t
	if err := s.save(); err != nil {
		s.chats[chatID] = old
		return err
	}

	return nil
}

// Language returns the language of a chat: Russian until the chat switches. A
// chat is known to the store or not, it makes no difference.
func (s *Store) Language(chatID int64) Language {
	s.mu.Lock()
	defer s.mu.Unlock()

	if lang, ok := s.languages[chatID]; ok {
		return lang
	}

	return Russian
}

// SetLanguage saves the language of a chat, subscribed or not. A failed save
// leaves the store as it was.
func (s *Store) SetLanguage(chatID int64, lang Language) error {
	if lang != Russian && lang != English {
		return fmt.Errorf("unknown language %q", lang)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old, hadOld := s.languages[chatID]
	if lang == Russian {
		delete(s.languages, chatID)
	} else {
		s.languages[chatID] = lang
	}

	if err := s.save(); err != nil {
		if hadOld {
			s.languages[chatID] = old
		} else {
			delete(s.languages, chatID)
		}
		return err
	}

	return nil
}

// Greeted reports whether the chat was sent the hello of its first /start.
func (s *Store) Greeted(chatID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.greeted[chatID]
}

// MarkGreeted records that the chat was sent the hello. A failed save leaves
// the store as it was.
func (s *Store) MarkGreeted(chatID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.greeted[chatID] {
		return nil
	}

	s.greeted[chatID] = true
	if err := s.save(); err != nil {
		delete(s.greeted, chatID)
		return err
	}

	return nil
}

// Due returns the subscribed chats whose delivery time is at or before now and
// that have not got the post on day, in ascending order.
func (s *Store) Due(now int, day string) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	var chats []int64
	for chatID, t := range s.chats {
		if t <= now && s.delivered[strconv.FormatInt(chatID, 10)] != day {
			chats = append(chats, chatID)
		}
	}
	slices.Sort(chats)

	return chats
}

// Known returns the chats the store knows: the subscribed ones and the ones
// that pressed Unsubscribe, in ascending order.
func (s *Store) Known() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	chats := s.sorted()
	for chatID := range s.optedOut {
		chats = append(chats, chatID)
	}
	slices.Sort(chats)

	return chats
}

// LastDelivered returns the last day chat got the post, or "" when the store
// has no record of one.
func (s *Store) LastDelivered(chat string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.delivered[chat]
}

// Delivered reports whether chat got the post on day.
func (s *Store) Delivered(chat, day string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.delivered[chat] == day
}

// MarkDelivered records that chat got the post on day. Records more than
// deliveredKeepDays older than day are dropped: for deciding who is due on day
// they mean the same as none, and this keeps chats that left from piling up in
// the file. The newer ones stay, so the last post day of a chat is known. A
// failed save keeps the record in memory, because the post did go out and must
// not go again.
func (s *Store) MarkDelivered(chat, day string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t, err := time.Parse(time.DateOnly, day); err == nil {
		oldest := t.AddDate(0, 0, -deliveredKeepDays).Format(time.DateOnly)
		for other, d := range s.delivered {
			if d < oldest {
				delete(s.delivered, other)
			}
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
	for chatID, lang := range s.languages {
		if data.Languages == nil {
			data.Languages = make(map[string]string)
		}
		data.Languages[strconv.FormatInt(chatID, 10)] = string(lang)
	}
	for chatID := range s.greeted {
		data.Greeted = append(data.Greeted, chatID)
	}
	slices.Sort(data.Greeted)
	for chatID, t := range s.chats {
		if t == DefaultTime {
			continue
		}
		key := strconv.FormatInt(chatID, 10)
		if data.Hours == nil {
			data.Hours = make(map[string]int)
		}
		data.Hours[key] = t / 60
		if t%60 != 0 {
			if data.Minutes == nil {
				data.Minutes = make(map[string]int)
			}
			data.Minutes[key] = t % 60
		}
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
