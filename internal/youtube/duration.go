// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var iso8601DurationRegex = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// ParseISO8601Duration converts an ISO 8601 duration string (e.g. "PT44M58S", "PT1H2M3S") into time.Duration.
func ParseISO8601Duration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	matches := iso8601DurationRegex.FindStringSubmatch(raw)
	if matches == nil {
		return 0, fmt.Errorf("invalid ISO 8601 duration %q", raw)
	}

	if matches[1] == "" && matches[2] == "" && matches[3] == "" && matches[4] == "" {
		return 0, fmt.Errorf("duration %q contains no components", raw)
	}

	var total time.Duration

	if daysStr := matches[1]; daysStr != "" {
		days, err := strconv.ParseInt(daysStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid days in duration: %w", err)
		}
		total += time.Duration(days) * 24 * time.Hour
	}

	if hoursStr := matches[2]; hoursStr != "" {
		hours, err := strconv.ParseInt(hoursStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hours in duration: %w", err)
		}
		total += time.Duration(hours) * time.Hour
	}

	if minsStr := matches[3]; minsStr != "" {
		mins, err := strconv.ParseInt(minsStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes in duration: %w", err)
		}
		total += time.Duration(mins) * time.Minute
	}

	if secsStr := matches[4]; secsStr != "" {
		secs, err := strconv.ParseFloat(secsStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid seconds in duration: %w", err)
		}
		total += time.Duration(secs * float64(time.Second))
	}

	return total, nil
}
