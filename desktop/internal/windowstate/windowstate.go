package windowstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type State struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

const (
	minWidth  = 800
	minHeight = 600
)

var ErrInvalid = errors.New("windowstate: invalid state")

func Load(path string) (State, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, false
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, false
	}
	if s.Width < minWidth || s.Height < minHeight {
		return State{}, false
	}
	return s, true
}

func Save(path string, s State) error {
	if s.Width < minWidth || s.Height < minHeight {
		return ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
