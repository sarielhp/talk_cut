package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
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
