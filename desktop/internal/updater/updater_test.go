package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckWithoutSetup(t *testing.T) {
	if info, err := Check(context.Background()); err == nil || info != nil {
		t.Fatalf("Check() = (%v, %v), want an error before Setup", info, err)
	}
}

func TestInstallerOperationsWithoutSetup(t *testing.T) {
	if err := Download(context.Background(), nil); err == nil {
		t.Fatal("Download() without Setup was accepted")
	}
	if err := Handoff(); err == nil {
		t.Fatal("Handoff() without Setup was accepted")
	}
	if err := Discard(); err == nil {
		t.Fatal("Discard() without Setup was accepted")
	}
}

func TestCleanStaleStagingKeepsRecentAndUnrelatedEntries(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, ".papersage-update-stale")
	recent := filepath.Join(root, ".papersage-update-in-flight")
	keepFile := filepath.Join(root, "keep.txt")
	keepDir := filepath.Join(root, "other")
	for _, dir := range []string{stale, recent, keepDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keepFile, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanStaleStaging(root)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale staging directory survived: %v", err)
	}
	for _, path := range []string{recent, keepFile, keepDir} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("cleanStaleStaging removed %q: %v", path, err)
		}
	}
}
