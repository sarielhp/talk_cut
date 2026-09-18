package youtube

import (
	"testing"
	"time"
)

func TestParseISO8601Duration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{input: "PT44M58S", expected: 44*time.Minute + 58*time.Second},
		{input: "PT1H2M3S", expected: 1*time.Hour + 2*time.Minute + 3*time.Second},
		{input: "PT30S", expected: 30 * time.Second},
		{input: "PT10M", expected: 10 * time.Minute},
		{input: "PT2H", expected: 2 * time.Hour},
		{input: "P1DT2H", expected: 26 * time.Hour},
		{input: "PT0S", expected: 0},
		{input: "PT1.5S", expected: 1500 * time.Millisecond},
		{input: "", wantErr: true},
		{input: "P", wantErr: true},
		{input: "invalid", wantErr: true},
		{input: "10:30", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseISO8601Duration(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseISO8601Duration(%q) expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseISO8601Duration(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.expected {
				t.Errorf("ParseISO8601Duration(%q) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}
