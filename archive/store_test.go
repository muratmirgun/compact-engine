package archive

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/muratmirgun/compact-engine/compact"
)

func TestStoreRoundTripAndIntegrity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	req := compact.Request{Goal: "fix", TargetTokens: 50, Messages: []compact.Message{{ID: "a", Role: "user", Text: "Türkçe 🧠"}}}
	id, err := s.Save(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(t.Context(), id)
	if err != nil || !reflect.DeepEqual(got, req) {
		t.Fatalf("Load()=%+v err=%v, want original request", got, err)
	}
	m, err := s.Recall(t.Context(), id, "a")
	if err != nil || m.Text != "Türkçe 🧠" {
		t.Fatalf("Recall()=%+v err=%v", m, err)
	}
	info, err := os.Stat(filepath.Join(dir, id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("snapshot permissions=%o, want 600", info.Mode().Perm())
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(t.Context(), id); err == nil {
		t.Error("Load(corrupt) succeeded")
	}
	if _, err := s.Load(t.Context(), "../../etc/passwd"); err == nil {
		t.Error("Load(traversal) succeeded")
	}
}

func TestConcurrentSnapshotWrites(t *testing.T) {
	t.Parallel()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req := compact.Request{Goal: "same snapshot"}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			id, err := s.Save(t.Context(), req)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := s.Load(t.Context(), id); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}
