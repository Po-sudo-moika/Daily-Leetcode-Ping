package subscribers

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestOpen_MissingFileIsEmpty(t *testing.T) {
	s, path := openTemp(t)

	if got := s.List(); len(got) != 0 {
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
	if !s.Has(42) {
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
	if s.Has(42) {
		t.Fatal("42 must not be subscribed")
	}
}

func TestListIsSorted(t *testing.T) {
	s, _ := openTemp(t)

	for _, id := range []int64{30, -100123, 5} {
		if _, err := s.Add(id); err != nil {
			t.Fatal(err)
		}
	}

	want := []int64{-100123, 5, 30}
	if got := s.List(); !reflect.DeepEqual(got, want) {
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
	if got := reopened.List(); !reflect.DeepEqual(got, want) {
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
	if s.Has(7) {
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
	if got := len(reopened.List()); got != 20 {
		t.Fatalf("got %d subscribers, want 20", got)
	}
}
