package main

import (
	"testing"
	"time"
)

func TestVerifyVideoLengthAgreement_Passes(t *testing.T) {
	tests := []struct {
		name     string
		localDur time.Duration
		ytDur    time.Duration
	}{
		{
			name:     "exact match",
			localDur: 2697 * time.Second,
			ytDur:    2697 * time.Second,
		},
		{
			name:     "real talk example (diff ~0.93s)",
			localDur: time.Duration(2697.068667 * float64(time.Second)),
			ytDur:    2698 * time.Second,
		},
		{
			name:     "minor delta (0.3s)",
			localDur: 100 * time.Second,
			ytDur:    time.Duration(100.3 * float64(time.Second)),
		},
		{
			name:     "boundary delta (1.0s)",
			localDur: 50 * time.Second,
			ytDur:    51 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyVideoLengthAgreement(tc.localDur, "test_cut.mp4", tc.ytDur, "GqfuUGTR12Y")
			if err != nil {
				t.Fatalf("expected pass for %s, got error: %v", tc.name, err)
			}
		})
	}
}

func TestVerifyVideoLengthAgreement_Fails(t *testing.T) {
	tests := []struct {
		name     string
		localDur time.Duration
		ytDur    time.Duration
	}{
		{
			name:     "exceeds delta (1.5s)",
			localDur: 100 * time.Second,
			ytDur:    time.Duration(101.5 * float64(time.Second)),
		},
		{
			name:     "large mismatch (10s)",
			localDur: 2697 * time.Second,
			ytDur:    2707 * time.Second,
		},
		{
			name:     "zero yt duration",
			localDur: 2697 * time.Second,
			ytDur:    0,
		},
		{
			name:     "zero local duration",
			localDur: 0,
			ytDur:    2698 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyVideoLengthAgreement(tc.localDur, "test_cut.mp4", tc.ytDur, "GqfuUGTR12Y")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestFormatMMSS(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{0, "00:00"},
		{59 * time.Second, "00:59"},
		{60 * time.Second, "01:00"},
		{2698 * time.Second, "44:58"},
		{3723 * time.Second, "62:03"},
	}

	for _, tc := range tests {
		got := formatMMSS(tc.d)
		if got != tc.expected {
			t.Errorf("formatMMSS(%v) = %q, want %q", tc.d, got, tc.expected)
		}
	}
}
