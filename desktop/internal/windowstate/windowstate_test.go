package windowstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	want := State{X: 120, Y: 80, Width: 1440, Height: 900, Maximised: true}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok := Load(path)
	if !ok {
		t.Fatal("Load reported no state")
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadRejectsMissingCorruptAndTooSmall(t *testing.T) {
	dir := t.TempDir()

	if _, ok := Load(filepath.Join(dir, "missing.json")); ok {
		t.Fatal("missing file should not load")
	}

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(corrupt); ok {
		t.Fatal("corrupt file should not load")
	}

	small := filepath.Join(dir, "small.json")
	if err := os.WriteFile(small, []byte(`{"width":200,"height":100}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(small); ok {
		t.Fatal("too-small window should not load")
	}
}

func TestSaveRejectsTooSmall(t *testing.T) {
	if err := Save(filepath.Join(t.TempDir(), "x.json"), State{Width: 10, Height: 10}); err == nil {
		t.Fatal("expected error")
	}
}
