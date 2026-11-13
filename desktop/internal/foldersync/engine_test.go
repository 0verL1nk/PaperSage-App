package foldersync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestEngine(t *testing.T) (*Engine, string, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "bound")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(dir, "bindings.json")
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	e := NewEngine(store)
	if err := e.Bind("p1", root); err != nil {
		t.Fatal(err)
	}
	return e, root, storePath
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBindScanApply(t *testing.T) {
	e, root, storePath := newTestEngine(t)

	changes, err := e.Scan("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("empty folder: got %v", changes)
	}

	content := []byte("\\documentclass{article}\n")
	if err := e.ApplyLocal("p1", "sections/main.tex", content); err != nil {
		t.Fatal(err)
	}
	got, err := e.ReadLocal("p1", "sections/main.tex")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("read back %q", got)
	}

	// ApplyLocal commits the baseline, so a rescan is clean.
	changes, err = e.Scan("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("after apply: got %v", changes)
	}

	// External edit shows up as Modified.
	write(t, root, "sections/main.tex", "changed\n")
	changes, err = e.Scan("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != ChangeModified || changes[0].Path != "sections/main.tex" {
		t.Fatalf("external edit: got %+v", changes)
	}
	if changes[0].Hash != hashBytes([]byte("changed\n")) {
		t.Fatalf("hash mismatch: %+v", changes[0])
	}

	if err := e.MarkSynced("p1", "sections/main.tex"); err != nil {
		t.Fatal(err)
	}
	changes, err = e.Scan("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("after mark synced: got %+v", changes)
	}

	// A fresh Store reads the persisted binding.
	store2, err := NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store2.Get("p1"); !ok {
		t.Fatal("binding was not persisted")
	}
}

func TestScanCreateDeleteRename(t *testing.T) {
	e, root, _ := newTestEngine(t)

	write(t, root, "main.tex", "one")
	changes, err := e.Scan("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != ChangeCreated || changes[0].Path != "main.tex" {
		t.Fatalf("create: got %+v", changes)
	}
	if err := e.MarkSynced("p1", "main.tex"); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(filepath.Join(root, "main.tex"), filepath.Join(root, "renamed.tex")); err != nil {
		t.Fatal(err)
	}
	changes, err = e.Scan("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("rename: got %+v", changes)
	}
	c := changes[0]
	if c.Kind != ChangeRenamed || c.Path != "renamed.tex" || c.OldPath != "main.tex" {
		t.Fatalf("rename shape: %+v", c)
	}

	// Commit the rename, then a plain delete is reported as Deleted.
	if err := e.MarkSynced("p1", "renamed.tex"); err != nil {
		t.Fatal(err)
	}
	if err := e.MarkSynced("p1", "main.tex"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "renamed.tex")); err != nil {
		t.Fatal(err)
	}
	changes, err = e.Scan("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != ChangeDeleted || changes[0].Path != "renamed.tex" {
		t.Fatalf("delete: got %+v", changes)
	}
	if changes[0].Hash != hashBytes([]byte("one")) {
		t.Fatalf("delete should carry last synced hash: %+v", changes[0])
	}
}

func TestApplyLocalConflict(t *testing.T) {
	e, root, _ := newTestEngine(t)

	if err := e.ApplyLocal("p1", "main.tex", []byte("base")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "main.tex", "local edit")

	err := e.ApplyLocal("p1", "main.tex", []byte("remote edit"))
	var conflict *Conflict
	if !errors.As(err, &conflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if conflict.Path != "main.tex" || conflict.BaseHash != hashBytes([]byte("base")) ||
		conflict.LocalHash != hashBytes([]byte("local edit")) ||
		conflict.RemoteHash != hashBytes([]byte("remote edit")) {
		t.Fatalf("conflict shape: %+v", conflict)
	}

	// The conflicting write must not touch disk.
	got, err := os.ReadFile(filepath.Join(root, "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "local edit" {
		t.Fatalf("disk was overwritten: %q", got)
	}

	// Content identical to disk is not a conflict.
	if err := e.ApplyLocal("p1", "main.tex", []byte("local edit")); err != nil {
		t.Fatalf("same content: %v", err)
	}
}

func TestScanIgnoreRules(t *testing.T) {
	e, root, _ := newTestEngine(t)
	write(t, root, "main.tex", "x")
	write(t, root, "main.aux", "x")
	write(t, root, "main.log", "x")
	write(t, root, "main.pdf", "x")
	write(t, root, "figure.png", "x")
	write(t, root, "notes.webp", "x")

	changes, err := e.Scan("p1")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Path] = true
	}
	if !seen["main.tex"] || !seen["figure.png"] {
		t.Fatalf("supported files missing: %+v", changes)
	}
	for _, rel := range []string{"main.aux", "main.log", "main.pdf", "notes.webp"} {
		if seen[rel] {
			t.Fatalf("%s should be ignored: %+v", rel, changes)
		}
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %+v", changes)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	e, root, _ := newTestEngine(t)
	outside := filepath.Join(filepath.Dir(root), "outside.tex")
	write(t, filepath.Dir(root), "outside.tex", "secret")

	for _, rel := range []string{"../outside.tex", "a/../../outside.tex", "/etc/passwd", `C:\secret.tex`, ""} {
		if err := e.ApplyLocal("p1", rel, []byte("x")); err == nil {
			t.Errorf("ApplyLocal(%q) should fail", rel)
		}
		if _, err := e.ReadLocal("p1", rel); err == nil {
			t.Errorf("ReadLocal(%q) should fail", rel)
		}
	}

	// Symlink pointing outside Root is rejected when supported (Windows needs
	// developer mode); otherwise the test is skipped.
	link := filepath.Join(root, "link.tex")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := e.ReadLocal("p1", "link.tex"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("outside symlink should be rejected, got %v", err)
	}
	if err := e.ApplyLocal("p1", "link.tex", []byte("x")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("write through outside symlink should be rejected, got %v", err)
	}
}

func TestAtomicWrite(t *testing.T) {
	e, root, _ := newTestEngine(t)

	if err := e.ApplyLocal("p1", "deep/nested/main.tex", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := e.ApplyLocal("p1", "deep/nested/main.tex", []byte("v2")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "deep", "nested", "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2" {
		t.Fatalf("content: %q", got)
	}

	var temps []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.Contains(filepath.Base(p), ".foldersync-") {
			temps = append(temps, p)
		}
		return nil
	})
	if len(temps) != 0 {
		t.Fatalf("temp files left behind: %v", temps)
	}
}

func TestWriteOverSizeRejected(t *testing.T) {
	e, _, _ := newTestEngine(t)
	if err := e.ApplyLocal("p1", "big.tex", make([]byte, MaxFileSize+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestUnbindStopsAndKeepsFiles(t *testing.T) {
	e, root, _ := newTestEngine(t)
	if err := e.ApplyLocal("p1", "main.tex", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := e.Unbind("p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Scan("p1"); !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "main.tex")); err != nil {
		t.Fatalf("unbind deleted files: %v", err)
	}
}

func TestWatchDebounce(t *testing.T) {
	e, root, _ := newTestEngine(t)

	got := make(chan []Change, 1)
	stop, err := e.Watch("p1", func(changes []Change) {
		select {
		case got <- changes:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	write(t, root, "main.tex", "external")

	select {
	case changes := <-got:
		if len(changes) != 1 || changes[0].Kind != ChangeCreated || changes[0].Path != "main.tex" {
			t.Fatalf("watch changes: %+v", changes)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for watch callback")
	}
}
