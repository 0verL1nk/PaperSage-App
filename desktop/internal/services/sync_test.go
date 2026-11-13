package services

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/0verL1nk/GraduateProject/desktop/internal/foldersync"
)

func TestSyncServiceBindScanWriteConflict(t *testing.T) {
	root := t.TempDir()
	storePath := filepath.Join(t.TempDir(), "sync.json")
	svc, err := NewSyncService(nil, storePath)
	if err != nil {
		t.Fatal(err)
	}
	const project = "p1"
	if err := svc.Bind(project, root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Unbind(project) }()

	if info := svc.Info(project); !info.Bound || info.Root == "" {
		t.Fatalf("expected bound info, got %+v", info)
	}

	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, err := svc.Scan(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != foldersync.ChangeCreated {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	encoded, err := svc.Read(project, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != "hello" {
		t.Fatalf("unexpected content: %q (%v)", decoded, err)
	}
	if err := svc.MarkSynced(project, "main.tex"); err != nil {
		t.Fatal(err)
	}
	if changes, err = svc.Scan(project); err != nil || len(changes) != 0 {
		t.Fatalf("expected clean scan, got %+v (%v)", changes, err)
	}

	if _, err := svc.Write(project, "main.tex", base64.StdEncoding.EncodeToString([]byte("v2"))); err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(filepath.Join(root, "main.tex"))
	if err != nil || string(disk) != "v2" {
		t.Fatalf("disk not updated: %q (%v)", disk, err)
	}

	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Write(project, "main.tex", base64.StdEncoding.EncodeToString([]byte("remote edit")))
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflict == nil {
		t.Fatal("expected a conflict instead of an overwrite")
	}
	if disk, _ = os.ReadFile(filepath.Join(root, "main.tex")); string(disk) != "local edit" {
		t.Fatalf("conflict must not overwrite the local file, got %q", disk)
	}

	reopened, err := NewSyncService(nil, storePath)
	if err != nil {
		t.Fatal(err)
	}
	if info := reopened.Info(project); !info.Bound || info.Files != 1 {
		t.Fatalf("baseline did not persist: %+v", info)
	}

	if missing, err := svc.Read(project, "gone.tex"); err != nil || missing != "" {
		t.Fatalf("missing file should return empty content, got %q (%v)", missing, err)
	}
}
