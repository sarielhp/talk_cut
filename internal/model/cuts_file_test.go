// Package model defines core domain data types for talk_cut.
package model

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCutsFile(t *testing.T) {
	tmpDir := t.TempDir()

	if HasSavedCuts(tmpDir) {
		t.Fatalf("expected HasSavedCuts to be false initially")
	}

	intervals := []CutInterval{
		{
			Start:  3 * time.Second,
			End:    25 * time.Second,
			Action: ActionCut,
			Reason: "Intro fluff",
		},
		{
			Start:  45 * time.Minute,
			End:    50 * time.Minute,
			Action: ActionCut,
			Reason: "Q&A",
		},
	}

	if err := SaveCutsFile(tmpDir, intervals); err != nil {
		t.Fatalf("SaveCutsFile failed: %v", err)
	}

	if !HasSavedCuts(tmpDir) {
		t.Fatalf("expected HasSavedCuts to be true after save")
	}

	loaded, err := LoadCutsFile(tmpDir)
	if err != nil {
		t.Fatalf("LoadCutsFile failed: %v", err)
	}

	if len(loaded) != len(intervals) {
		t.Fatalf("expected %d intervals, got %d", len(intervals), len(loaded))
	}

	for i := range intervals {
		if loaded[i].Start != intervals[i].Start || loaded[i].End != intervals[i].End {
			t.Errorf("interval %d mismatch: %+v vs %+v", i, loaded[i], intervals[i])
		}
		if loaded[i].Action != intervals[i].Action || loaded[i].Reason != intervals[i].Reason {
			t.Errorf("interval %d properties mismatch: %+v vs %+v", i, loaded[i], intervals[i])
		}
	}
}

func TestLoadCutsFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := LoadCutsFile(filepath.Join(tmpDir, "nonexistent"))
	if err == nil {
		t.Errorf("expected error loading non-existent cuts file")
	}
}
