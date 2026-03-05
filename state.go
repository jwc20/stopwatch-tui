package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const stateFileName = ".stopwatch-state.json"

type AppState struct {
	Running   bool      `json:"running"`
	ElapsedNs int64     `json:"elapsed_ns"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Splits    []int64   `json:"splits,omitempty"`
}

func stateFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, stateFileName), nil
}

func LoadState() (*AppState, error) {
	path, err := stateFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func SaveState(state AppState) error {
	path, err := stateFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func DeleteState() error {
	path, err := stateFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *AppState) ElapsedDuration() time.Duration {
	elapsed := time.Duration(s.ElapsedNs)
	if s.Running {
		elapsed += time.Since(s.StartedAt)
	}
	return elapsed
}

func (s *AppState) SplitDurations() []time.Duration {
	splits := make([]time.Duration, len(s.Splits))
	for i, ns := range s.Splits {
		splits[i] = time.Duration(ns)
	}
	return splits
}
