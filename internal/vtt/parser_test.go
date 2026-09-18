package vtt

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"00:00:03.830", 3*time.Second + 830*time.Millisecond, false},
		{"01:02:03.450", 1*time.Hour + 2*time.Minute + 3*time.Second + 450*time.Millisecond, false},
		{"02:15.500", 2*time.Minute + 15*time.Second + 500*time.Millisecond, false},
		{"00:00:05,200", 5*time.Second + 200*time.Millisecond, false},
		{"00:00:01", 1 * time.Second, false},
		{"", 0, true},
		{"invalid", 0, true},
		{"00:99:99:99", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseTimestamp(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseTimestamp(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("ParseTimestamp(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatTimestamp(t *testing.T) {
	dur := 1*time.Hour + 23*time.Minute + 45*time.Second + 678*time.Millisecond
	got := FormatTimestamp(dur)
	if got != "01:23:45.678" {
		t.Errorf("FormatTimestamp(%v) = %q, want %q", dur, got, "01:23:45.678")
	}

	short := FormatTimestampShort(dur)
	if short != "83:45" {
		t.Errorf("FormatTimestampShort(%v) = %q, want %q", dur, short, "83:45")
	}

	shortMin := FormatTimestampShort(4*time.Minute + 12*time.Second)
	if shortMin != "04:12" {
		t.Errorf("FormatTimestampShort(4m12s) = %q, want %q", shortMin, "04:12")
	}
}

func TestParseVTT(t *testing.T) {
	raw := `WEBVTT - Test Recording

NOTE This is a comment block
that spans multiple lines

1
00:00:03.830 --> 00:00:04.529 line:0%
Boris Aronov: Go ahead.

2
00:00:04.530 --> 00:00:05.150
Adam Sheffer: Great!

3
00:00:05.380 --> 00:00:07.549
<v Karim Abu Affash>Hi everyone, <b>thank you</b> for coming.</v>
`

	cues, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(cues) != 3 {
		t.Fatalf("expected 3 cues, got %d", len(cues))
	}

	if cues[0].Speaker != "Boris Aronov" || cues[0].Text != "Go ahead." {
		t.Errorf("cue 0 mismatch: %+v", cues[0])
	}
	if cues[0].End != 4*time.Second+529*time.Millisecond {
		t.Errorf("cue 0 end timestamp mismatch: %v", cues[0].End)
	}

	if cues[1].Speaker != "Adam Sheffer" || cues[1].Text != "Great!" {
		t.Errorf("cue 1 mismatch: %+v", cues[1])
	}

	if cues[2].Speaker != "Karim Abu Affash" || cues[2].Text != "Hi everyone, thank you for coming." {
		t.Errorf("cue 2 mismatch: %+v", cues[2])
	}
}

func TestParseRealExampleVTT(t *testing.T) {
	examplePath := "../../examples/26_09_08/GMT20260908-180238_Recording.transcript.vtt"
	file, err := os.Open(examplePath)
	if err != nil {
		t.Skip("sample example vtt not found on disk, skipping integration test")
		return
	}
	defer file.Close()

	cues, err := Parse(file)
	if err != nil {
		t.Fatalf("Parse on real sample failed: %v", err)
	}

	if len(cues) != 540 {
		t.Fatalf("expected 540 cues from real sample, got %d", len(cues))
	}

	if cues[0].Speaker != "Boris Aronov" {
		t.Errorf("first speaker mismatch: %q", cues[0].Speaker)
	}
	if cues[len(cues)-1].Speaker != "Boris Aronov" {
		t.Errorf("last speaker mismatch: %q", cues[len(cues)-1].Speaker)
	}
}

func TestWriteRoundtrip(t *testing.T) {
	raw := `WEBVTT

1
00:00:01.000 --> 00:00:03.000
Speaker A: Hello world

2
00:00:03.500 --> 00:00:06.000
Speaker B: Second line
`

	cues, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	var buf bytes.Buffer
	if writeErr := Write(&buf, cues); writeErr != nil {
		t.Fatalf("Write failed: %v", writeErr)
	}

	cues2, parseErr := Parse(&buf)
	if parseErr != nil {
		t.Fatalf("re-parsing written VTT failed: %v", parseErr)
	}

	if len(cues2) != 2 {
		t.Fatalf("expected 2 cues after roundtrip, got %d", len(cues2))
	}
	if cues2[0].Text != cues[0].Text || cues2[0].Speaker != cues[0].Speaker {
		t.Errorf("cue 0 roundtrip mismatch: got %+v, want %+v", cues2[0], cues[0])
	}
}
