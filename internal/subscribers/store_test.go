package subscribers

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "nested", "subscribers.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	return s, path
}

// today is the Moscow day the tests deliver on.
const today = "2026-10-11"

// subscribed returns every subscribed chat in ascending order: with nothing
// delivered today, everyone is due at the last hour.
func subscribed(s *Store) []int64 {
	return s.Due(MaxHour, today)
}

func TestOpen_MissingFileIsEmpty(t *testing.T) {
	s, path := openTemp(t)

	if got := subscribed(s); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("open must not create the file, stat err: %v", err)
	}
}

func TestAddRemove(t *testing.T) {
	s, _ := openTemp(t)

	added, err := s.Add(42)
	if err != nil || !added {
		t.Fatalf("first add: added=%v err=%v", added, err)
	}
	added, err = s.Add(42)
	if err != nil || added {
		t.Fatalf("second add: added=%v err=%v", added, err)
	}
	if _, ok := s.Hour(42); !ok {
		t.Fatal("42 must be subscribed")
	}

	removed, err := s.Remove(42)
	if err != nil || !removed {
		t.Fatalf("first remove: removed=%v err=%v", removed, err)
	}
	removed, err = s.Remove(42)
	if err != nil || removed {
		t.Fatalf("second remove: removed=%v err=%v", removed, err)
	}
	if _, ok := s.Hour(42); ok {
		t.Fatal("42 must not be subscribed")
	}
}

func TestDueIsSorted(t *testing.T) {
	s, _ := openTemp(t)

	for _, id := range []int64{30, -100123, 5} {
		if _, err := s.Add(id); err != nil {
			t.Fatal(err)
		}
	}

	want := []int64{-100123, 5, 30}
	if got := s.Due(DefaultHour, today); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSurvivesRestart(t *testing.T) {
	s, path := openTemp(t)

	for _, id := range []int64{1, 2, 3} {
		if _, err := s.Add(id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Remove(2); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	want := []int64{1, 3}
	if got := subscribed(reopened); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSaveLeavesNoTempFiles(t *testing.T) {
	s, path := openTemp(t)

	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "subscribers.json" {
		t.Fatalf("unexpected files: %v", entries)
	}
}

func TestOpen_CorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscribers.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(path); err == nil {
		t.Fatal("want an error for a corrupt file")
	}
}

func TestAdd_RollsBackWhenSaveFails(t *testing.T) {
	dir := t.TempDir()

	s, err := Open(filepath.Join(dir, "sub", "subscribers.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// A regular file where the directory should be makes every save fail.
	if err := os.WriteFile(filepath.Join(dir, "sub"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Add(7); err == nil {
		t.Fatal("want a save error")
	}
	if _, ok := s.Hour(7); ok {
		t.Fatal("a failed add must not stay in memory")
	}
}

func TestConcurrentAdd(t *testing.T) {
	s, path := openTemp(t)

	var wg sync.WaitGroup
	for i := int64(1); i <= 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Add(i); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(subscribed(reopened)); got != 20 {
		t.Fatalf("got %d subscribers, want 20", got)
	}
}

// oldFormat is a subscribers.json as written before delivery hours existed.
const oldFormat = `{
  "chats": [
    -100123,
    5,
    30
  ]
}`

func writeFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "subscribers.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestOpen_OldFormatKeepsEveryoneAtDefaultHour(t *testing.T) {
	s, err := Open(writeFile(t, oldFormat))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	want := []int64{-100123, 5, 30}
	if got := subscribed(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("subscribers %v, want %v", got, want)
	}
	for _, chatID := range want {
		if hour, ok := s.Hour(chatID); !ok || hour != DefaultHour {
			t.Errorf("chat %d: hour=%d ok=%v, want %d", chatID, hour, ok, DefaultHour)
		}
	}
	if got := s.Due(DefaultHour, today); !reflect.DeepEqual(got, want) {
		t.Fatalf("due at default hour %v, want %v", got, want)
	}
	if got := s.Due(DefaultHour-1, today); len(got) != 0 {
		t.Fatalf("due before the default hour %v, want nobody", got)
	}
}

func TestOldFormat_SurvivesFirstSaveAndStaysReadableByOldBuilds(t *testing.T) {
	path := writeFile(t, oldFormat)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Add(99); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHour(5, 9); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// What the previous build reads: only the chats list.
	var old struct {
		Chats []int64 `json:"chats"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	if want := []int64{-100123, 5, 30, 99}; !reflect.DeepEqual(old.Chats, want) {
		t.Fatalf("chats in file %v, want %v", old.Chats, want)
	}
}

func TestSetHour(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(2); err != nil {
		t.Fatal(err)
	}

	if hour, ok := s.Hour(1); !ok || hour != DefaultHour {
		t.Fatalf("new subscriber: hour=%d ok=%v, want default %d", hour, ok, DefaultHour)
	}

	if err := s.SetHour(1, 8); err != nil {
		t.Fatal(err)
	}
	if got := s.Due(8, today); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("due at 8: %v, want [1]", got)
	}
	if got := s.Due(DefaultHour, today); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("due at default: %v, want [1 2]", got)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if hour, _ := reopened.Hour(1); hour != 8 {
		t.Fatalf("hour after restart %d, want 8", hour)
	}
	if hour, _ := reopened.Hour(2); hour != DefaultHour {
		t.Fatalf("hour of other chat after restart %d, want %d", hour, DefaultHour)
	}
}

func TestSetHour_Range(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}

	for _, hour := range []int{MinHour, MaxHour, DefaultHour} {
		if err := s.SetHour(1, hour); err != nil {
			t.Errorf("hour %d must be allowed: %v", hour, err)
		}
	}
	// 03:00 Moscow is when LeetCode changes the problem, so it is not offered.
	for _, hour := range []int{-1, 0, 3, MinHour - 1, MaxHour + 1, 24} {
		if err := s.SetHour(1, hour); !errors.Is(err, ErrInvalidHour) {
			t.Errorf("hour %d: err=%v, want ErrInvalidHour", hour, err)
		}
	}
	if hour, _ := s.Hour(1); hour != DefaultHour {
		t.Fatalf("a rejected hour must not change the choice, got %d", hour)
	}
}

func TestSetHour_NotSubscribed(t *testing.T) {
	s, _ := openTemp(t)

	if err := s.SetHour(1, 9); !errors.Is(err, ErrNotSubscribed) {
		t.Fatalf("err=%v, want ErrNotSubscribed", err)
	}
	if _, ok := s.Hour(1); ok {
		t.Fatal("SetHour must not subscribe")
	}
}

func TestSetHour_RollsBackWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "sub", "subscribers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}

	// Replace the directory with a regular file: every later save fails.
	if err := os.RemoveAll(filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.SetHour(1, 9); err == nil {
		t.Fatal("want a save error")
	}
	if hour, _ := s.Hour(1); hour != DefaultHour {
		t.Fatalf("a failed save must not change the hour, got %d", hour)
	}
}

func TestRemoveThenAddResetsHour(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHour(1, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remove(1); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "hours") {
		t.Fatalf("a removed subscriber must leave no hour behind:\n%s", raw)
	}

	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}
	if hour, _ := s.Hour(1); hour != DefaultHour {
		t.Fatalf("resubscribed chat has hour %d, want %d", hour, DefaultHour)
	}
}

func TestOpen_IgnoresStrayHours(t *testing.T) {
	path := writeFile(t, `{"chats":[1,2],"hours":{"1":9,"2":3,"3":9}}`)

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if hour, _ := s.Hour(1); hour != 9 {
		t.Errorf("chat 1 hour %d, want 9", hour)
	}
	if hour, _ := s.Hour(2); hour != DefaultHour {
		t.Errorf("chat 2 has an out-of-range hour, want default, got %d", hour)
	}
	if _, ok := s.Hour(3); ok {
		t.Error("an hour entry must not subscribe chat 3")
	}
}

func TestOpen_BadHoursKey(t *testing.T) {
	if _, err := Open(writeFile(t, `{"chats":[1],"hours":{"abc":9}}`)); err == nil {
		t.Fatal("want an error for a non-numeric chat id")
	}
}

func TestDue_SkipsChatsDeliveredThatDay(t *testing.T) {
	s, _ := openTemp(t)
	for _, id := range []int64{1, 2} {
		if _, err := s.Add(id); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.MarkDelivered("1", today); err != nil {
		t.Fatal(err)
	}

	if got := s.Due(MaxHour, today); !reflect.DeepEqual(got, []int64{2}) {
		t.Fatalf("due today %v, want [2]", got)
	}
	if got := s.Due(MaxHour, "2026-10-12"); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("due the next day %v, want [1 2]", got)
	}
}

// Deliveries to subscribers and to fixed chats from config.toml are read back
// after a restart.
func TestMarkDelivered_SurvivesRestart(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}
	for _, chat := range []string{"1", "@news"} {
		if err := s.MarkDelivered(chat, today); err != nil {
			t.Fatal(err)
		}
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, chat := range []string{"1", "@news"} {
		if !reopened.Delivered(chat, today) {
			t.Errorf("%s: delivery lost on restart", chat)
		}
	}
	if reopened.Delivered("2", today) {
		t.Error("chat 2 never got the post")
	}
	if got := reopened.Due(MaxHour, today); len(got) != 0 {
		t.Fatalf("due after restart %v, want nobody", got)
	}
}

func TestMarkDelivered_DropsOlderDays(t *testing.T) {
	s, path := openTemp(t)
	if err := s.MarkDelivered("@old", "2026-10-10"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDelivered("@news", today); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data fileData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"@news": today}; !reflect.DeepEqual(data.Delivered, want) {
		t.Fatalf("deliveries in file %v, want %v", data.Delivered, want)
	}
}

// The post did go out, so a failed save must not make the chat due again.
func TestMarkDelivered_KeepsRecordWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "sub", "subscribers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.MarkDelivered("@news", today); err == nil {
		t.Fatal("want a save error")
	}
	if !s.Delivered("@news", today) {
		t.Fatal("the delivery must stay recorded")
	}
}

// A file from before deliveries were recorded loads with nobody delivered,
// keeps its hours, and after the first delivery an older build still reads
// the chats.
func TestOldFormats_LoadWithNothingDelivered(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		hour5   int
	}{
		"chats only":      {oldFormat, DefaultHour},
		"chats and hours": {`{"chats":[-100123,5,30],"hours":{"5":9}}`, 9},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeFile(t, tc.content)
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}

			want := []int64{-100123, 5, 30}
			if got := s.Due(MaxHour, today); !reflect.DeepEqual(got, want) {
				t.Fatalf("due %v, want everyone %v", got, want)
			}

			if err := s.MarkDelivered("5", today); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := reopened.Due(MaxHour, today); !reflect.DeepEqual(got, []int64{-100123, 30}) {
				t.Fatalf("due after a delivery and a restart %v, want [-100123 30]", got)
			}
			if hour, _ := reopened.Hour(5); hour != tc.hour5 {
				t.Fatalf("hour of chat 5 %d, want %d", hour, tc.hour5)
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var old struct {
				Chats []int64 `json:"chats"`
			}
			if err := json.Unmarshal(raw, &old); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(old.Chats, want) {
				t.Fatalf("chats in file %v, want %v", old.Chats, want)
			}
		})
	}
}
