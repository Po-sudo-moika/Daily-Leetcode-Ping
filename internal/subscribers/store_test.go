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
	return s.Due(MaxTime, today)
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
	if _, ok := s.Time(42); !ok {
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
	if _, ok := s.Time(42); ok {
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
	if got := s.Due(DefaultTime, today); !reflect.DeepEqual(got, want) {
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
	if _, ok := s.Time(7); ok {
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
		if hour, ok := s.Time(chatID); !ok || hour != DefaultTime {
			t.Errorf("chat %d: hour=%d ok=%v, want %d", chatID, hour, ok, DefaultHour)
		}
	}
	if got := s.Due(DefaultTime, today); !reflect.DeepEqual(got, want) {
		t.Fatalf("due at default hour %v, want %v", got, want)
	}
	if got := s.Due(DefaultTime-1, today); len(got) != 0 {
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
	if err := s.SetTime(5, At(9, 0)); err != nil {
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

func TestSetTime(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(2); err != nil {
		t.Fatal(err)
	}

	if got, ok := s.Time(1); !ok || got != DefaultTime {
		t.Fatalf("new subscriber: time=%d ok=%v, want default %d", got, ok, DefaultTime)
	}

	if err := s.SetTime(1, At(8, 0)); err != nil {
		t.Fatal(err)
	}
	if got := s.Due(At(8, 0), today); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("due at 8: %v, want [1]", got)
	}
	if got := s.Due(DefaultTime, today); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("due at default: %v, want [1 2]", got)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.Time(1); got != At(8, 0) {
		t.Fatalf("time after restart %d, want 08:00", got)
	}
	if got, _ := reopened.Time(2); got != DefaultTime {
		t.Fatalf("time of other chat after restart %d, want %d", got, DefaultTime)
	}
}

func TestSetTime_MinutesAreDueExactlyAtTheirMinute(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTime(1, At(9, 30)); err != nil {
		t.Fatal(err)
	}

	if got := s.Due(At(9, 29), today); len(got) != 0 {
		t.Fatalf("due at 09:29 %v, want nobody", got)
	}
	if got := s.Due(At(9, 30), today); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("due at 09:30 %v, want [1]", got)
	}
	if err := s.MarkDelivered("1", today); err != nil {
		t.Fatal(err)
	}
	if got := s.Due(At(9, 31), today); len(got) != 0 {
		t.Fatalf("due after the post %v, want nobody", got)
	}

	reopened := reopen(t, path)
	if got, _ := reopened.Time(1); got != At(9, 30) {
		t.Fatalf("time after restart %s, want 09:30", Format(got))
	}
	if got := reopened.Due(At(9, 30), "2026-10-12"); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("due the next day at 09:30 %v, want [1]", got)
	}
}

func TestSetTime_Range(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}

	for _, ok := range []int{MinTime, MaxTime, DefaultTime, At(9, 5), At(23, 55)} {
		if err := s.SetTime(1, ok); err != nil {
			t.Errorf("%s must be allowed: %v", Format(ok), err)
		}
	}
	// 03:00 Moscow is when LeetCode changes the problem, so it is not offered.
	// Minutes are in steps of five.
	for _, bad := range []int{-1, 0, At(3, 0), At(3, 55), MinTime - 5, MaxTime + 5, At(24, 0), At(9, 1), At(9, 59), At(10, 7)} {
		if err := s.SetTime(1, bad); !errors.Is(err, ErrInvalidTime) {
			t.Errorf("time %d: err=%v, want ErrInvalidTime", bad, err)
		}
	}
	if got, _ := s.Time(1); got != At(23, 55) {
		t.Fatalf("a rejected time must not change the choice, got %s", Format(got))
	}
}

func TestSetTime_NotSubscribed(t *testing.T) {
	s, _ := openTemp(t)

	if err := s.SetTime(1, At(9, 0)); !errors.Is(err, ErrNotSubscribed) {
		t.Fatalf("err=%v, want ErrNotSubscribed", err)
	}
	if _, ok := s.Time(1); ok {
		t.Fatal("SetHour must not subscribe")
	}
}

func TestSetTime_RollsBackWhenSaveFails(t *testing.T) {
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

	if err := s.SetTime(1, At(9, 0)); err == nil {
		t.Fatal("want a save error")
	}
	if hour, _ := s.Time(1); hour != DefaultTime {
		t.Fatalf("a failed save must not change the hour, got %d", hour)
	}
}

func TestRemoveThenAddResetsHour(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Add(1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTime(1, At(9, 0)); err != nil {
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
	if hour, _ := s.Time(1); hour != DefaultTime {
		t.Fatalf("resubscribed chat has hour %d, want %d", hour, DefaultHour)
	}
}

func TestOpen_IgnoresStrayHours(t *testing.T) {
	path := writeFile(t, `{"chats":[1,2],"hours":{"1":9,"2":3,"3":9}}`)

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if hour, _ := s.Time(1); hour != At(9, 0) {
		t.Errorf("chat 1 hour %d, want 9", hour)
	}
	if hour, _ := s.Time(2); hour != DefaultTime {
		t.Errorf("chat 2 has an out-of-range hour, want default, got %d", hour)
	}
	if _, ok := s.Time(3); ok {
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

	if got := s.Due(MaxTime, today); !reflect.DeepEqual(got, []int64{2}) {
		t.Fatalf("due today %v, want [2]", got)
	}
	if got := s.Due(MaxTime, "2026-10-12"); !reflect.DeepEqual(got, []int64{1, 2}) {
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
	if got := reopened.Due(MaxTime, today); len(got) != 0 {
		t.Fatalf("due after restart %v, want nobody", got)
	}
}

func TestMarkDelivered_KeepsRecentDaysAndDropsVeryOldOnes(t *testing.T) {
	s, path := openTemp(t)
	for chat, day := range map[string]string{"@ancient": "2026-06-01", "@recent": "2026-10-10"} {
		if err := s.MarkDelivered(chat, day); err != nil {
			t.Fatal(err)
		}
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
	if want := map[string]string{"@recent": "2026-10-10", "@news": today}; !reflect.DeepEqual(data.Delivered, want) {
		t.Fatalf("deliveries in file %v, want %v", data.Delivered, want)
	}
	if got := s.LastDelivered("@recent"); got != "2026-10-10" {
		t.Fatalf("last post of @recent %q, want 2026-10-10", got)
	}
	if got := s.LastDelivered("@ancient"); got != "" {
		t.Fatalf("last post of a dropped record %q, want none", got)
	}
	// An older record never makes a chat due: it only differs from today.
	if err := s.MarkDelivered("7", "2026-10-10"); err != nil {
		t.Fatal(err)
	}
	if s.Delivered("7", today) {
		t.Fatal("a record of an older day must not count as today's post")
	}
}

func TestKnown_ListsSubscribersAndChatsThatUnsubscribed(t *testing.T) {
	s, _ := openTemp(t)
	for _, chatID := range []int64{30, -100123, 5} {
		if _, err := s.Add(chatID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Remove(30); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Meet(8); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Drop(8); err != nil {
		t.Fatal(err)
	}

	if got, want := s.Known(), []int64{-100123, 5, 30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("known %v, want %v (a dropped chat is forgotten)", got, want)
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
			if got := s.Due(MaxTime, today); !reflect.DeepEqual(got, want) {
				t.Fatalf("due %v, want everyone %v", got, want)
			}

			if err := s.MarkDelivered("5", today); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := reopened.Due(MaxTime, today); !reflect.DeepEqual(got, []int64{-100123, 30}) {
				t.Fatalf("due after a delivery and a restart %v, want [-100123 30]", got)
			}
			if hour, _ := reopened.Time(5); hour != At(tc.hour5, 0) {
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

// reopen loads the store again from its file, like a restart.
func reopen(t *testing.T, path string) *Store {
	t.Helper()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	return s
}

func TestMeet_SubscribesAChatSeenForTheFirstTime(t *testing.T) {
	s, path := openTemp(t)

	met, err := s.Meet(42)
	if err != nil || !met {
		t.Fatalf("first meet: met=%v err=%v", met, err)
	}
	if hour, ok := s.Time(42); !ok || hour != DefaultTime {
		t.Fatalf("hour=%d ok=%v, want subscribed at %d", hour, ok, DefaultHour)
	}

	if _, ok := reopen(t, path).Time(42); !ok {
		t.Fatal("the subscription must survive a restart")
	}
}

func TestMeet_KeepsTheHourOfASubscriber(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := s.Add(42); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTime(42, At(9, 0)); err != nil {
		t.Fatal(err)
	}

	met, err := s.Meet(42)
	if err != nil || met {
		t.Fatalf("meet of a subscriber: met=%v err=%v", met, err)
	}
	if hour, _ := s.Time(42); hour != At(9, 0) {
		t.Fatalf("hour %d, want 9", hour)
	}
}

func TestMeet_RespectsUnsubscribe_AlsoAfterRestart(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Meet(42); err != nil {
		t.Fatal(err)
	}
	if removed, err := s.Remove(42); err != nil || !removed {
		t.Fatalf("remove: removed=%v err=%v", removed, err)
	}

	for name, store := range map[string]*Store{"same run": s, "after restart": reopen(t, path)} {
		met, err := store.Meet(42)
		if err != nil || met {
			t.Fatalf("%s: meet: met=%v err=%v", name, met, err)
		}
		if _, ok := store.Time(42); ok {
			t.Fatalf("%s: an unsubscribed chat must stay out", name)
		}
	}
}

func TestRemove_OfAnUnknownChatStillRecordsTheChoice(t *testing.T) {
	s, path := openTemp(t)

	if removed, err := s.Remove(42); err != nil || removed {
		t.Fatalf("remove: removed=%v err=%v", removed, err)
	}
	if met, err := reopen(t, path).Meet(42); err != nil || met {
		t.Fatalf("meet after restart: met=%v err=%v", met, err)
	}
}

func TestAdd_SubscribesAgainAfterUnsubscribe(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Remove(42); err != nil {
		t.Fatal(err)
	}

	if added, err := s.Add(42); err != nil || !added {
		t.Fatalf("add: added=%v err=%v", added, err)
	}

	restarted := reopen(t, path)
	if _, ok := restarted.Time(42); !ok {
		t.Fatal("subscribed again, it must stay subscribed after a restart")
	}
	// Unsubscribing and subscribing again leaves no stale opt-out behind.
	if _, err := restarted.Drop(42); err != nil {
		t.Fatal(err)
	}
	if met, err := restarted.Meet(42); err != nil || !met {
		t.Fatalf("meet after drop: met=%v err=%v", met, err)
	}
}

func TestDrop_UnsubscribesButLetsTheChatComeBack(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Meet(42); err != nil {
		t.Fatal(err)
	}

	if dropped, err := s.Drop(42); err != nil || !dropped {
		t.Fatalf("drop: dropped=%v err=%v", dropped, err)
	}
	if dropped, err := s.Drop(42); err != nil || dropped {
		t.Fatalf("second drop: dropped=%v err=%v", dropped, err)
	}
	if _, ok := reopen(t, path).Time(42); ok {
		t.Fatal("a dropped chat must stay out after a restart")
	}

	// It never pressed Unsubscribe, so a new contact subscribes it again.
	if met, err := s.Meet(42); err != nil || !met {
		t.Fatalf("meet after drop: met=%v err=%v", met, err)
	}
}

func TestDrop_KeepsTheChoiceOfAChatThatUnsubscribed(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := s.Remove(42); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Drop(42); err != nil {
		t.Fatal(err)
	}
	if met, err := s.Meet(42); err != nil || met {
		t.Fatalf("meet: met=%v err=%v; the unsubscribe choice must survive a drop", met, err)
	}
}

func TestMeetRemoveDrop_RollBackWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "sub", "subscribers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Meet(7); err == nil {
		t.Fatal("meet: want a save error")
	}
	if _, ok := s.Time(7); ok {
		t.Fatal("a failed meet must not stay in memory")
	}

	if _, err := s.Remove(8); err == nil {
		t.Fatal("remove: want a save error")
	}
}

func TestMigrate_UpgradedGroupKeepsItsChoice(t *testing.T) {
	const (
		optedOutGroup, optedOutSupergroup     = int64(-1), int64(-1001)
		subscribedGroup, subscribedSupergroup = int64(-2), int64(-1002)
	)
	s, path := openTemp(t)
	if _, err := s.Remove(optedOutGroup); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(subscribedGroup); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTime(subscribedGroup, At(15, 0)); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDelivered("-2", today); err != nil {
		t.Fatal(err)
	}

	for from, to := range map[int64]int64{optedOutGroup: optedOutSupergroup, subscribedGroup: subscribedSupergroup} {
		if moved, err := s.Migrate(from, to); err != nil || !moved {
			t.Fatalf("migrate %d: moved=%v err=%v, want moved", from, moved, err)
		}
	}

	restarted, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for name, store := range map[string]*Store{"before restart": s, "after restart": restarted} {
		if met, err := store.Meet(optedOutSupergroup); err != nil || met {
			t.Fatalf("%s: met=%v err=%v, the opted-out supergroup must stay out", name, met, err)
		}
		if hour, ok := store.Time(subscribedSupergroup); !ok || hour != At(15, 0) {
			t.Fatalf("%s: hour=%d subscribed=%v, want the supergroup at 15", name, hour, ok)
		}
		if !store.Delivered("-1002", today) {
			t.Fatalf("%s: the supergroup must keep today's delivery", name)
		}
		if _, ok := store.Time(subscribedGroup); ok {
			t.Fatalf("%s: the old id must be gone", name)
		}
	}
}

func TestMigrate_UnknownChatChangesNothing(t *testing.T) {
	s, _ := openTemp(t)

	if moved, err := s.Migrate(-1, -1001); err != nil || moved {
		t.Fatalf("moved=%v err=%v, want nothing moved", moved, err)
	}
	if met, _ := s.Meet(-1001); !met {
		t.Fatal("a new id with nothing moved to it is met as usual")
	}
}

func TestMigrate_RollsBackWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "sub", "subscribers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(-1); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "sub", "subscribers.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Migrate(-1, -1001); err == nil {
		t.Fatal("want a save error")
	}
	if _, ok := s.Time(-1); !ok {
		t.Fatal("a failed migrate must keep the old id")
	}
	if _, ok := s.Time(-1001); ok {
		t.Fatal("a failed migrate must not subscribe the new id")
	}
}

// A file written before unsubscribed chats were recorded loads unchanged.
func TestOpen_FileWithoutOptOutsLoadsUnchanged(t *testing.T) {
	s, err := Open(writeFile(t, `{"chats":[5,30],"hours":{"5":9},"delivered":{"5":"`+today+`"}}`))
	if err != nil {
		t.Fatal(err)
	}

	if hour, ok := s.Time(5); !ok || hour != At(9, 0) {
		t.Fatalf("chat 5: hour=%d ok=%v, want 9", hour, ok)
	}
	if hour, ok := s.Time(30); !ok || hour != DefaultTime {
		t.Fatalf("chat 30: hour=%d ok=%v, want %d", hour, ok, DefaultHour)
	}
	if !s.Delivered("5", today) {
		t.Fatal("delivery record lost")
	}
}

func TestSave_KeepsChatsReadableByThePreviousBuild(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.Add(5); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remove(6); err != nil {
		t.Fatal(err)
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
	if want := []int64{5}; !reflect.DeepEqual(old.Chats, want) {
		t.Fatalf("chats in file %v, want %v", old.Chats, want)
	}
}

// A file written before minutes existed keeps every chat at its hour, :00.
func TestOpen_FileWithoutMinutesLoadsEveryChatAtItsHourSharp(t *testing.T) {
	s, err := Open(writeFile(t, `{"chats":[5,30,31],"hours":{"5":9,"31":23}}`))
	if err != nil {
		t.Fatal(err)
	}

	for chatID, want := range map[int64]int{5: At(9, 0), 30: At(11, 0), 31: At(23, 0)} {
		if got, ok := s.Time(chatID); !ok || got != want {
			t.Errorf("chat %d: time=%s ok=%v, want %s", chatID, Format(got), ok, Format(want))
		}
	}
	if got := s.Due(At(9, 0), today); !reflect.DeepEqual(got, []int64{5}) {
		t.Errorf("due at 09:00 %v, want [5]", got)
	}
}

func TestSetTime_MinutesOfTheDefaultHourSurviveARestart(t *testing.T) {
	s, path := openTemp(t)
	for chatID, at := range map[int64]int{1: At(11, 5), 2: At(4, 0), 3: At(23, 55), 4: At(11, 0)} {
		if _, err := s.Add(chatID); err != nil {
			t.Fatal(err)
		}
		if err := s.SetTime(chatID, at); err != nil {
			t.Fatal(err)
		}
	}

	reopened := reopen(t, path)
	for chatID, want := range map[int64]int{1: At(11, 5), 2: At(4, 0), 3: At(23, 55), 4: At(11, 0)} {
		if got, _ := reopened.Time(chatID); got != want {
			t.Errorf("chat %d: time=%s, want %s", chatID, Format(got), Format(want))
		}
	}
}

func TestOpen_IgnoresStrayMinutes(t *testing.T) {
	s, err := Open(writeFile(t, `{"chats":[1,2,3],"hours":{"1":9,"2":9},"minutes":{"1":30,"2":7,"3":99,"4":30}}`))
	if err != nil {
		t.Fatal(err)
	}

	for chatID, want := range map[int64]int{1: At(9, 30), 2: At(9, 0), 3: At(11, 0)} {
		if got, _ := s.Time(chatID); got != want {
			t.Errorf("chat %d: time=%s, want %s", chatID, Format(got), Format(want))
		}
	}
	if _, ok := s.Time(4); ok {
		t.Error("a minute entry must not subscribe chat 4")
	}
}

func TestOpen_BadMinutesKey(t *testing.T) {
	if _, err := Open(writeFile(t, `{"chats":[1],"minutes":{"abc":30}}`)); err == nil {
		t.Fatal("want an error for a chat id that is not a number")
	}
}

func TestFormat(t *testing.T) {
	for at, want := range map[int]string{At(9, 30): "09:30", At(23, 55): "23:55", MinTime: "04:00", DefaultTime: "11:00"} {
		if got := Format(at); got != want {
			t.Errorf("Format(%d)=%q, want %q", at, got, want)
		}
	}
}
