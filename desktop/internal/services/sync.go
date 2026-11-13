package services

import (
	"encoding/base64"
	"errors"
	"sync"

	"github.com/0verL1nk/GraduateProject/desktop/internal/foldersync"
)

// SyncService exposes the local folder engine to the web layer. All remote
// API calls stay in the frontend; this service only touches disk.
type SyncService struct {
	emit   func(name string, data any)
	store  *foldersync.Store
	engine *foldersync.Engine

	mu      sync.Mutex
	watches map[string]func()
}

// SyncBindingInfo summarises one project binding for the UI.
type SyncBindingInfo struct {
	Bound  bool   `json:"bound"`
	Root   string `json:"root,omitempty"`
	Paused bool   `json:"paused"`
	Files  int    `json:"files"`
}

// SyncWriteResult reports a conflict instead of overwriting local edits.
type SyncWriteResult struct {
	Conflict *foldersync.Conflict `json:"conflict,omitempty"`
}

// NewSyncService loads bindings from storePath. emit forwards sync events to
// the web layer.
func NewSyncService(emit func(name string, data any), storePath string) (*SyncService, error) {
	store, err := foldersync.NewStore(storePath)
	if err != nil {
		return nil, err
	}
	return &SyncService{
		emit:    emit,
		store:   store,
		engine:  foldersync.NewEngine(store),
		watches: map[string]func(){},
	}, nil
}

// ServiceName is the stable binding name used by the frontend.
func (s *SyncService) ServiceName() string { return "SyncService" }

// ResumeWatches restarts filesystem watches for existing bindings.
func (s *SyncService) ResumeWatches() {
	for _, binding := range s.store.List() {
		_ = s.startWatch(binding.ProjectID)
	}
}

// Info returns the binding state for one project.
func (s *SyncService) Info(projectID string) SyncBindingInfo {
	binding, ok := s.store.Get(projectID)
	if !ok {
		return SyncBindingInfo{}
	}
	return SyncBindingInfo{
		Bound:  true,
		Root:   binding.Root,
		Paused: binding.Paused,
		Files:  len(binding.LastSynced),
	}
}

// Bind links a project to a local folder and starts watching it.
func (s *SyncService) Bind(projectID, root string) error {
	if err := s.engine.Bind(projectID, root); err != nil {
		return err
	}
	s.stopWatch(projectID)
	return s.startWatch(projectID)
}

// Unbind removes the project binding and its watch.
func (s *SyncService) Unbind(projectID string) error {
	s.stopWatch(projectID)
	return s.engine.Unbind(projectID)
}

// SetPaused pauses or resumes disk watching and scans.
func (s *SyncService) SetPaused(projectID string, paused bool) error {
	if err := s.engine.SetPaused(projectID, paused); err != nil {
		return err
	}
	if paused {
		s.stopWatch(projectID)
		return nil
	}
	s.stopWatch(projectID)
	return s.startWatch(projectID)
}

// Scan reports disk changes relative to the last synced baseline.
func (s *SyncService) Scan(projectID string) ([]foldersync.Change, error) {
	return s.engine.Scan(projectID)
}

// Read returns a local file as base64; an empty string means the file is gone.
func (s *SyncService) Read(projectID, rel string) (string, error) {
	data, err := s.engine.ReadLocal(projectID, rel)
	if err != nil {
		if errors.Is(err, foldersync.ErrNotBound) || errors.Is(err, foldersync.ErrPaused) {
			return "", err
		}
		return "", nil
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// Write applies application content to disk. A conflict is reported, not
// overwritten.
func (s *SyncService) Write(projectID, rel, dataBase64 string) (SyncWriteResult, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return SyncWriteResult{}, err
	}
	err = s.engine.ApplyLocal(projectID, rel, data)
	var conflict *foldersync.Conflict
	if errors.As(err, &conflict) {
		return SyncWriteResult{Conflict: conflict}, nil
	}
	if err != nil {
		return SyncWriteResult{}, err
	}
	return SyncWriteResult{}, nil
}

// MarkSynced records that a disk change has been applied to the application.
func (s *SyncService) MarkSynced(projectID, rel string) error {
	return s.engine.MarkSynced(projectID, rel)
}

// Watch starts emitting desktop:sync-change for this project (idempotent).
func (s *SyncService) Watch(projectID string) error {
	s.mu.Lock()
	_, exists := s.watches[projectID]
	s.mu.Unlock()
	if exists {
		return nil
	}
	return s.startWatch(projectID)
}

// Unwatch stops the filesystem watcher for this project.
func (s *SyncService) Unwatch(projectID string) {
	s.stopWatch(projectID)
}

func (s *SyncService) startWatch(projectID string) error {
	stop, err := s.engine.Watch(projectID, func(changes []foldersync.Change) {
		if s.emit != nil {
			s.emit("desktop:sync-change", map[string]any{
				"projectId": projectID,
				"changes":   changes,
			})
		}
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.watches[projectID] = stop
	s.mu.Unlock()
	return nil
}

func (s *SyncService) stopWatch(projectID string) {
	s.mu.Lock()
	stop := s.watches[projectID]
	delete(s.watches, projectID)
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
}
