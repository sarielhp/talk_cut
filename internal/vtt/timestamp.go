// Package vtt provides WebVTT subtitle parsing and timestamp formatting.
package vtt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseTimestamp parses WebVTT timestamp strings ("HH:MM:SS.mmm" or "MM:SS.mmm") into time.Duration.
func ParseTimestamp(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty timestamp string")
	}

	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid timestamp format: %q", s)
	}

	var hours int64
	var minStr, secStr string

	if len(parts) == 3 {
		h, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hours in timestamp %q: %w", s, err)
		}
		hours = h
		minStr = parts[1]
		secStr = parts[2]
	} else {
		minStr = parts[0]
		secStr = parts[1]
	}

	minutes, err := strconv.ParseInt(minStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in timestamp %q: %w", s, err)
	}

	seconds, millis, err := parseSecondsAndMillis(secStr)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in timestamp %q: %w", s, err)
	}

	dur := time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(millis)*time.Millisecond

	return dur, nil
}

// parseSecondsAndMillis splits seconds and milliseconds ("SS.mmm" or "SS,mmm").
func parseSecondsAndMillis(secStr string) (int64, int64, error) {
	secStr = strings.ReplaceAll(secStr, ",", ".")
	secParts := strings.Split(secStr, ".")

	seconds, err := strconv.ParseInt(secParts[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	if len(secParts) == 1 {
		return seconds, 0, nil
	}

	rawMillis := secParts[1]
	if len(rawMillis) > 3 {
		rawMillis = rawMillis[:3]
	}
	for len(rawMillis) < 3 {
		rawMillis += "0"
	}

	millis, err := strconv.ParseInt(rawMillis, 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return seconds, millis, nil
}

// FormatTimestamp returns a WebVTT timestamp formatted as "HH:MM:SS.mmm".
func FormatTimestamp(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	totalMillis := d.Milliseconds()
	hours := totalMillis / 3600000
	totalMillis %= 3600000
	minutes := totalMillis / 60000
	totalMillis %= 60000
	seconds := totalMillis / 1000
	millis := totalMillis % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

// FormatTimestampShort formats duration for display without milliseconds: "HH:MM:SS" or "MM:SS".
func FormatTimestampShort(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	totalSeconds := int64(d.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
