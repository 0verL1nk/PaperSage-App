package foldersync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchDebounce = 500 * time.Millisecond

var (
	ErrNotBound = errors.New("foldersync: project not bound")
	ErrPaused   = errors.New("foldersync: sync paused")
	ErrTooLarge = errors.New("foldersync: file too large")
)

// ChangeKind describes how a file differs from the last synced baseline.
type ChangeKind string

const (
	ChangeCreated  ChangeKind = "created"
	ChangeModified ChangeKind = "modified"
	ChangeDeleted  ChangeKind = "deleted"
	ChangeRenamed  ChangeKind = "renamed"
)

// Change is one difference between disk and the last synced baseline.
// Renamed changes carry the previous path in OldPath; deleted changes carry the
// last synced hash and no size or mod time.
type Change struct {
	Kind    ChangeKind `json:"kind"`
	Path    string     `json:"path"`
	OldPath string     `json:"oldPath,omitempty"`
	Hash    string     `json:"hash,omitempty"`
	Size    int64      `json:"size,omitempty"`
	ModTime time.Time  `json:"modTime"`
}

// Conflict reports a file that changed on both sides since the last sync.
// ApplyLocal returns it instead of overwriting; the caller resolves it.
type Conflict struct {
	Path       string `json:"path"`
	BaseHash   string `json:"baseHash,omitempty"`
	LocalHash  string `json:"localHash"`
	RemoteHash string `json:"remoteHash"`
}

func (c *Conflict) Error() string {
	return fmt.Sprintf("foldersync: conflict on %s", c.Path)
}

// Engine binds project folders to local directories and reconciles them with a
// persistent baseline. Scan never mutates the baseline: call MarkSynced after a
// reported change has been applied on the other side. ApplyLocal is the
// app-to-disk path and updates the baseline itself.
type Engine struct {
	store *Store

	mu       sync.Mutex
	nextID   uint64
	watchers map[uint64]watcher
}

type watcher struct {
	projectID string
	stop      func()
}

// NewEngine returns an engine backed by store.
func NewEngine(store *Store) *Engine {
	return &Engine{store: store, watchers: map[uint64]watcher{}}
}

// Bind binds projectID to root, resolving symlinks first. Rebinding the same
// root keeps the baseline; binding a different root starts a fresh one.
func (e *Engine) Bind(projectID, root string) error {
	if projectID == "" {
		return errors.New("foldersync: empty project id")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	info, err := os.Stat(real)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("foldersync: %s is not a directory", root)
	}

	last := map[string]FileState{}
	if prev, ok := e.store.Get(projectID); ok && prev.Root == real {
		last = prev.LastSynced
	}
	e.stopWatchers(projectID)
	return e.store.Put(Binding{ProjectID: projectID, Root: real, LastSynced: last})
}

// Unbind removes the binding and stops its watchers. Files are left untouched.
func (e *Engine) Unbind(projectID string) error {
	e.stopWatchers(projectID)
	return e.store.Delete(projectID)
}

// SetPaused toggles automatic scanning and watching for a binding.
func (e *Engine) SetPaused(projectID string, paused bool) error {
	return e.store.mutate(projectID, func(b *Binding) error {
		b.Paused = paused
		return nil
	})
}

// Scan compares the bound folder with LastSynced. It returns created, modified,
// deleted and renamed changes sorted by path. Renames are inferred from a
// delete and a create with identical content; deletions and renames are
// reported only, never propagated.
func (e *Engine) Scan(projectID string) ([]Change, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	b, ok := e.store.Get(projectID)
	if !ok {
		return nil, ErrNotBound
	}
	if b.Paused {
		return nil, ErrPaused
	}
	current, err := scanDir(b.Root)
	if err != nil {
		return nil, err
	}

	var created, modified, deleted []Change
	for rel, f := range current {
		base, seen := b.LastSynced[rel]
		c := Change{Path: rel, Hash: f.Hash, Size: f.Size, ModTime: f.ModTime}
		switch {
		case !seen:
			c.Kind = ChangeCreated
			created = append(created, c)
		case base.Hash != f.Hash:
			c.Kind = ChangeModified
			modified = append(modified, c)
		}
	}
	for rel, base := range b.LastSynced {
		if _, ok := current[rel]; !ok {
			deleted = append(deleted, Change{Kind: ChangeDeleted, Path: rel, Hash: base.Hash})
		}
	}

	sortChanges(created)
	sortChanges(deleted)
	changes := pairRenames(created, deleted)
	changes = append(changes, modified...)
	sortChanges(changes)
	return changes, nil
}

// ReadLocal reads a supported file from the bound folder after path checks.
func (e *Engine) ReadLocal(projectID, rel string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	b, ok := e.store.Get(projectID)
	if !ok {
		return nil, ErrNotBound
	}
	rel, err := SafeRelPath(rel)
	if err != nil {
		return nil, err
	}
	if !IsSupported(rel) || IsIgnored(filepath.Base(rel)) {
		return nil, ErrUnsupported
	}
	full, err := safePath(b.Root, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrUnsupported
	}
	if info.Size() > MaxFileSize {
		return nil, ErrTooLarge
	}
	return os.ReadFile(full)
}

// ApplyLocal writes app-side content to the bound folder atomically. If both
// the file on disk and content differ from LastSynced it returns a *Conflict
// and leaves the disk untouched.
func (e *Engine) ApplyLocal(projectID, rel string, content []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	b, ok := e.store.Get(projectID)
	if !ok {
		return ErrNotBound
	}
	rel, err := SafeRelPath(rel)
	if err != nil {
		return err
	}
	if !IsSupported(rel) || IsIgnored(filepath.Base(rel)) {
		return ErrUnsupported
	}
	if len(content) > MaxFileSize {
		return ErrTooLarge
	}
	full, err := safePath(b.Root, rel)
	if err != nil {
		return err
	}

	incoming := hashBytes(content)
	var diskHash string
	var diskExists bool
	if info, err := os.Stat(full); err == nil {
		if !info.Mode().IsRegular() {
			return ErrUnsupported
		}
		if info.Size() > MaxFileSize {
			return ErrTooLarge
		}
		diskExists = true
		if diskHash, _, err = hashFile(full); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	base := b.LastSynced[rel].Hash
	if diskExists && diskHash == incoming {
		return e.setBaseline(projectID, rel, incoming, full)
	}
	if diskExists && diskHash != base && incoming != base {
		return &Conflict{Path: rel, BaseHash: base, LocalHash: diskHash, RemoteHash: incoming}
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := ensureWithin(b.Root, full); err != nil {
		return err
	}
	if err := writeAtomic(full, content); err != nil {
		return err
	}
	return e.setBaseline(projectID, rel, incoming, full)
}

// MarkSynced records the current disk state of rel as the baseline. If the
// file is gone, its baseline entry is removed. Call this after a change has
// been applied on the other side.
func (e *Engine) MarkSynced(projectID, rel string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	b, ok := e.store.Get(projectID)
	if !ok {
		return ErrNotBound
	}
	rel, err := SafeRelPath(rel)
	if err != nil {
		return err
	}
	full, err := safePath(b.Root, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return e.store.mutate(projectID, func(b *Binding) error {
			delete(b.LastSynced, rel)
			return nil
		})
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrUnsupported
	}
	if info.Size() > MaxFileSize {
		return ErrTooLarge
	}
	hash, _, err := hashFile(full)
	if err != nil {
		return err
	}
	return e.setBaseline(projectID, rel, hash, full)
}

// Watch observes the bound folder and calls onChange with the changes found
// after a 500ms debounce. Only one callback can be in flight at a time.
func (e *Engine) Watch(projectID string, onChange func([]Change)) (func(), error) {
	b, ok := e.store.Get(projectID)
	if !ok {
		return nil, ErrNotBound
	}
	if b.Paused {
		return nil, ErrPaused
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := addRecursive(w, b.Root); err != nil {
		w.Close()
		return nil, err
	}

	done := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(done)
			w.Close()
		})
	}
	id := e.addWatcher(projectID, stop)

	go func() {
		defer e.removeWatcher(id)
		var timer *time.Timer
		var timerC <-chan time.Time
		defer func() {
			if timer != nil {
				timer.Stop()
			}
		}()

		for {
			select {
			case <-done:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
						_ = addRecursive(w, ev.Name)
					}
				}
				if !relevantEvent(b.Root, ev.Name) {
					continue
				}
				if timer == nil {
					timer = time.NewTimer(watchDebounce)
				} else {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(watchDebounce)
				}
				timerC = timer.C
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			case <-timerC:
				timerC = nil
				changes, err := e.Scan(projectID)
				if err == nil && len(changes) > 0 && onChange != nil {
					onChange(changes)
				}
			}
		}
	}()

	return stop, nil
}

func (e *Engine) setBaseline(projectID, rel, hash, path string) error {
	state, err := stateOf(path, hash)
	if err != nil {
		return err
	}
	return e.store.mutate(projectID, func(b *Binding) error {
		b.LastSynced[rel] = state
		return nil
	})
}

func (e *Engine) addWatcher(projectID string, stop func()) uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextID++
	e.watchers[e.nextID] = watcher{projectID: projectID, stop: stop}
	return e.nextID
}

func (e *Engine) removeWatcher(id uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.watchers, id)
}

func (e *Engine) stopWatchers(projectID string) {
	e.mu.Lock()
	var stops []func()
	for id, w := range e.watchers {
		if w.projectID == projectID {
			stops = append(stops, w.stop)
			delete(e.watchers, id)
		}
	}
	e.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
}

type scannedFile struct {
	Hash    string
	ModTime time.Time
	Size    int64
}

func scanDir(root string) (map[string]scannedFile, error) {
	out := map[string]scannedFile{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if p == root {
				return walkErr
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, err := SafeRelPath(rel); err != nil {
			return nil
		}
		if !IsSupported(rel) || IsIgnored(filepath.Base(rel)) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			real, err := filepath.EvalSymlinks(p)
			if err != nil || !withinRoot(root, real) {
				return nil
			}
			if info, err = os.Stat(p); err != nil {
				return nil
			}
		}
		if !info.Mode().IsRegular() || info.Size() > MaxFileSize {
			return nil
		}
		hash, mod, err := hashFile(p)
		if err != nil {
			return nil
		}
		out[rel] = scannedFile{Hash: hash, ModTime: mod, Size: info.Size()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func pairRenames(created, deleted []Change) []Change {
	free := map[string][]int{}
	for i, c := range created {
		free[c.Hash] = append(free[c.Hash], i)
	}
	used := make([]bool, len(created))
	out := make([]Change, 0, len(created)+len(deleted))
	for _, d := range deleted {
		paired := -1
		for _, i := range free[d.Hash] {
			if !used[i] {
				used[i] = true
				paired = i
				break
			}
		}
		if paired < 0 {
			out = append(out, d)
			continue
		}
		c := created[paired]
		c.Kind = ChangeRenamed
		c.OldPath = d.Path
		out = append(out, c)
	}
	for i, c := range created {
		if !used[i] {
			out = append(out, c)
		}
	}
	return out
}

func sortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
}

func hashFile(path string) (string, time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return "", time.Time{}, err
	}
	if n > MaxFileSize {
		return "", time.Time{}, ErrTooLarge
	}
	info, err := f.Stat()
	if err != nil {
		return "", time.Time{}, err
	}
	return hex.EncodeToString(h.Sum(nil)), info.ModTime().UTC(), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func stateOf(path, hash string) (FileState, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileState{}, err
	}
	return FileState{Hash: hash, ModTime: info.ModTime().UTC()}, nil
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".foldersync-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// safePath validates rel and resolves symlinks in the existing part of the
// path, rejecting anything that leaves root.
func safePath(root, rel string) (string, error) {
	rel, err := SafeRelPath(rel)
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	if !withinRoot(root, full) {
		return "", ErrUnsafePath
	}
	if err := ensureWithin(root, full); err != nil {
		return "", err
	}
	return full, nil
}

func ensureWithin(root, full string) error {
	real, err := resolveExisting(full)
	if err != nil {
		return err
	}
	if !withinRoot(root, real) {
		return ErrUnsafePath
	}
	return nil
}

// resolveExisting resolves symlinks in the deepest existing ancestor and
// appends the still-missing suffix.
func resolveExisting(p string) (string, error) {
	cur := p
	suffix := ""
	for {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(real, suffix), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p, nil
		}
		suffix = filepath.Join(filepath.Base(cur), suffix)
		cur = parent
	}
}

func withinRoot(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func relevantEvent(root, name string) bool {
	rel, err := filepath.Rel(root, name)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if _, err := SafeRelPath(rel); err != nil {
		return false
	}
	return !IsIgnored(filepath.Base(rel))
}

func addRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		if d.IsDir() {
			if err := w.Add(p); err != nil && p == root {
				return err
			}
		}
		return nil
	})
}
