package foldersync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Binding is one project's bound folder and its last-synced baseline.
type Binding struct {
	ProjectID  string               `json:"projectId"`
	Root       string               `json:"root"`
	Paused     bool                 `json:"paused"`
	LastSynced map[string]FileState `json:"lastSynced"`
}

// FileState is the content hash and modification time recorded at the last sync.
type FileState struct {
	Hash    string    `json:"hash"`
	ModTime time.Time `json:"modTime"`
}

// Store persists bindings as one JSON file.
type Store struct {
	path     string
	mu       sync.Mutex
	bindings map[string]*Binding
}

// NewStore loads bindings from path. A missing file starts empty.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path, bindings: map[string]*Binding{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.bindings); err != nil {
		return nil, fmt.Errorf("foldersync: load store: %w", err)
	}
	for id, b := range s.bindings {
		if b == nil {
			delete(s.bindings, id)
			continue
		}
		b.ProjectID = id
		if b.LastSynced == nil {
			b.LastSynced = map[string]FileState{}
		}
	}
	return s, nil
}

// Get returns a copy of the binding for projectID.
func (s *Store) Get(projectID string) (Binding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bindings[projectID]
	if !ok {
		return Binding{}, false
	}
	return b.clone(), true
}

// List returns copies of all bindings.
func (s *Store) List() []Binding {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Binding, 0, len(s.bindings))
	for _, b := range s.bindings {
		out = append(out, b.clone())
	}
	return out
}

// Put inserts or replaces a binding and flushes it to disk.
func (s *Store) Put(b Binding) error {
	if b.ProjectID == "" {
		return errors.New("foldersync: empty project id")
	}
	c := b.clone()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[c.ProjectID] = &c
	return s.flushLocked()
}

// Delete removes a binding and flushes the store.
func (s *Store) Delete(projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bindings[projectID]; !ok {
		return nil
	}
	delete(s.bindings, projectID)
	return s.flushLocked()
}

// mutate applies fn to an existing binding and flushes the store.
func (s *Store) mutate(projectID string, fn func(*Binding) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bindings[projectID]
	if !ok {
		return ErrNotBound
	}
	if err := fn(b); err != nil {
		return err
	}
	return s.flushLocked()
}

func (s *Store) flushLocked() error {
	if s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(s.bindings, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
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
	if err := os.Rename(name, s.path); err != nil {
		// Redirected AppData (junctions, packaged-app redirection) can make a
		// same-directory rename look cross-device; fall back to an in-place
		// write so bindings still persist.
		if writeErr := os.WriteFile(s.path, data, 0o600); writeErr != nil {
			return err
		}
	}
	return nil
}

func (b Binding) clone() Binding {
	out := b
	out.LastSynced = make(map[string]FileState, len(b.LastSynced))
	for k, v := range b.LastSynced {
		out.LastSynced[k] = v
	}
	return out
}
