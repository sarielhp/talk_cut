package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
)

func captureStdout(f func() error) (string, error) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := f()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), err
}

func TestRunYouTube_DetailedHelp(t *testing.T) {
	tests := [][]string{
		{"youtube", "setup", "-H"},
		{"youtube", "-H"},
		{"youtube", "--guide"},
	}

	for _, args := range tests {
		output, err := captureStdout(func() error {
			return run(args)
		})
		if err != nil {
			t.Fatalf("run(%v) error: %v", args, err)
		}

		if !strings.Contains(output, "TALK_CUT YOUTUBE SETUP GUIDE") {
			t.Errorf("run(%v) output did not contain guide title: %s", args, output)
		}
		if !strings.Contains(output, "gcloud services enable youtube.googleapis.com") {
			t.Errorf("run(%v) output did not contain gcloud instructions: %s", args, output)
		}
	}
}

func TestRunYouTube_UsageAndStatus(t *testing.T) {
	// Test basic youtube usage
	output, err := captureStdout(func() error {
		return run([]string{"youtube"})
	})
	if err != nil {
		t.Fatalf("run(youtube) error: %v", err)
	}
	if !strings.Contains(output, "talk_cut youtube setup [-H]") {
		t.Errorf("run(youtube) output did not contain setup command: %s", output)
	}

	// Test youtube status
	statusOutput, err := captureStdout(func() error {
		return run([]string{"youtube", "status"})
	})
	if err != nil {
		t.Fatalf("run(youtube status) error: %v", err)
	}
	if !strings.Contains(statusOutput, "=== YouTube Configuration Status ===") {
		t.Errorf("run(youtube status) did not contain header: %s", statusOutput)
	}

	// Test unknown command
	err = run([]string{"youtube", "invalid_subcommand"})
	if err == nil {
		t.Errorf("expected error for invalid subcommand, got nil")
	}
}

func TestRunUpload_InvalidDir(t *testing.T) {
	err := run([]string{"--upload", "nonexistent_dir_for_test"})
	if err == nil {
		t.Errorf("expected error when running --upload on nonexistent dir, got nil")
	}
	if !strings.Contains(err.Error(), "is not a valid directory") {
		t.Errorf("expected invalid directory error, got: %v", err)
	}
}

func TestHasEmptyMetadataFields(t *testing.T) {
	dir := "/path/to/26/09/15"

	// All empty / default
	meta1 := model.TalkMetadata{
		Title: "15", // equals filepath.Base(dir)
	}
	if !hasEmptyMetadataFields(meta1, dir) {
		t.Errorf("expected true for default title and empty speaker/abstract")
	}

	// Completely empty title
	meta2 := model.TalkMetadata{}
	if !hasEmptyMetadataFields(meta2, dir) {
		t.Errorf("expected true for empty struct")
	}

	// Non-default title
	meta3 := model.TalkMetadata{
		Title: "A Great Talk on Geometry",
	}
	if hasEmptyMetadataFields(meta3, dir) {
		t.Errorf("expected false when title is customized")
	}

	// Speaker set
	meta4 := model.TalkMetadata{
		Title:   "15",
		Speaker: "Alice Smith",
	}
	if hasEmptyMetadataFields(meta4, dir) {
		t.Errorf("expected false when speaker is set")
	}

	// Abstract set
	meta5 := model.TalkMetadata{
		Title:    "15",
		Abstract: "This talk discusses algorithms.",
	}
	if hasEmptyMetadataFields(meta5, dir) {
		t.Errorf("expected false when abstract is set")
	}
}

func TestInitialMetadata_PreservesExistingEdits(t *testing.T) {
	tempDir := t.TempDir()
	savedMeta := model.TalkMetadata{
		Title:    "Manually Curated Title",
		Speaker:  "Curated Speaker",
		Abstract: "Curated abstract.",
		Privacy:  "unlisted",
	}
	if err := model.SaveMetaFile(tempDir, savedMeta); err != nil {
		t.Fatalf("saving meta file: %v", err)
	}

	emlPath := filepath.Join(tempDir, "announcement.eml")
	emlContent := "Subject: Different Email Title\r\n\r\nEmail body content"
	if err := os.WriteFile(emlPath, []byte(emlContent), 0644); err != nil {
		t.Fatalf("writing dummy eml: %v", err)
	}

	opts := &cliOptions{
		dir:      tempDir,
		noAI:     false,
		reDetect: false,
		metaOnly: false,
	}
	cfg := config.Config{}

	meta := initialMetadata(context.Background(), opts, cfg)
	if meta.Title != "Manually Curated Title" {
		t.Errorf("expected Title to be preserved as %q, got %q", "Manually Curated Title", meta.Title)
	}
	if meta.Speaker != "Curated Speaker" {
		t.Errorf("expected Speaker to be preserved as %q, got %q", "Curated Speaker", meta.Speaker)
	}
}
