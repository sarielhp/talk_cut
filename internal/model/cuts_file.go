// Package model defines core domain data types for talk_cut.
package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CutsFileName is the standard filename for persisted cut state in a recording directory.
const CutsFileName = "talk_cuts.json"

// SavedCut defines the JSON serialization for an individual interval cut.
type SavedCut struct {
	Start     string        `json:"start"`
	End       string        `json:"end"`
	StartNano time.Duration `json:"start_nano"`
	EndNano   time.Duration `json:"end_nano"`
	Action    string        `json:"action"`
	Reason    string        `json:"reason,omitempty"`
}

// SavedCutsFile represents the top-level persisted cuts document.
type SavedCutsFile struct {
	UpdatedAt string     `json:"updated_at"`
	Cuts      []SavedCut `json:"cuts"`
}

// SaveCutsFile writes the cut intervals to <dir>/talk_cuts.json.
func SaveCutsFile(dir string, intervals []CutInterval) error {
	path := filepath.Join(dir, CutsFileName)
	scf := SavedCutsFile{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	for _, inv := range intervals {
		scf.Cuts = append(scf.Cuts, SavedCut{
			Start:     inv.Start.String(),
			End:       inv.End.String(),
			StartNano: inv.Start,
			EndNano:   inv.End,
			Action:    inv.Action.String(),
			Reason:    inv.Reason,
		})
	}

	data, err := json.MarshalIndent(scf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cuts: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing cuts file %q: %w", path, err)
	}

	return nil
}

// HasSavedCuts checks if <dir>/talk_cuts.json exists.
func HasSavedCuts(dir string) bool {
	path := filepath.Join(dir, CutsFileName)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// LoadCutsFile loads persisted cut intervals from <dir>/talk_cuts.json.
func LoadCutsFile(dir string) ([]CutInterval, error) {
	path := filepath.Join(dir, CutsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cuts file %q: %w", path, err)
	}

	var scf SavedCutsFile
	if err := json.Unmarshal(data, &scf); err != nil {
		return nil, fmt.Errorf("unmarshaling cuts file %q: %w", path, err)
	}

	var intervals []CutInterval
	for _, sc := range scf.Cuts {
		action := ActionKeep
		if sc.Action == "CUT" {
			action = ActionCut
		} else if sc.Action == "REVIEW" {
			action = ActionReview
		}

		intervals = append(intervals, CutInterval{
			Start:  sc.StartNano,
			End:    sc.EndNano,
			Action: action,
			Reason: sc.Reason,
		})
	}

	return intervals, nil
}
