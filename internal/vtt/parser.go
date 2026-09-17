// Package vtt provides WebVTT subtitle parsing and timestamp formatting.
package vtt

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"talk_cut/internal/model"
)

var (
	tagRegex     = regexp.MustCompile(`<[^>]+>`)
	voiceTagRe   = regexp.MustCompile(`^<v\s+([^>]+)>(.*?)(?:</v>)?$`)
	speakerColon = regexp.MustCompile(`^([A-Z0-9][A-Za-z0-9 .,'\-_/()]+?):\s+(.*)$`)
)

type rawCue struct {
	idLines   string
	timeLine  string
	textLines []string
}

// Parse parses WebVTT content from an io.Reader into a slice of SubtitleCue models.
func Parse(r io.Reader) ([]model.SubtitleCue, error) {
	scanner := bufio.NewScanner(r)
	var cues []model.SubtitleCue
	var current rawCue
	inNote := false

	cueIndex := 1
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if inNote {
			if line == "" {
				inNote = false
			}
			continue
		}

		if strings.HasPrefix(line, "NOTE") {
			inNote = true
			continue
		}

		if strings.HasPrefix(line, "WEBVTT") {
			continue
		}

		if line == "" {
			if current.timeLine != "" {
				cue, err := buildCue(current, cueIndex)
				if err != nil {
					return nil, fmt.Errorf("parsing cue #%d: %w", cueIndex, err)
				}
				cues = append(cues, cue)
				cueIndex++
				current = rawCue{}
			}
			continue
		}

		processCueLine(&current, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning vtt: %w", err)
	}

	if current.timeLine != "" {
		cue, err := buildCue(current, cueIndex)
		if err != nil {
			return nil, fmt.Errorf("parsing final cue #%d: %w", cueIndex, err)
		}
		cues = append(cues, cue)
	}

	return cues, nil
}

// processCueLine routes incoming non-empty lines into the current cue buffer.
func processCueLine(c *rawCue, line string) {
	if strings.Contains(line, "-->") {
		c.timeLine = line
		return
	}

	if c.timeLine == "" {
		c.idLines = line
		return
	}

	c.textLines = append(c.textLines, line)
}

// buildCue parses timings and text for a collected raw cue block.
func buildCue(raw rawCue, fallbackIndex int) (model.SubtitleCue, error) {
	start, end, err := parseTimingLine(raw.timeLine)
	if err != nil {
		return model.SubtitleCue{}, err
	}

	speaker, text := extractSpeakerAndText(raw.textLines)

	id := fallbackIndex
	if raw.idLines != "" {
		if parsedID, err := strconv.Atoi(raw.idLines); err == nil {
			id = parsedID
		}
	}

	return model.SubtitleCue{
		ID:      id,
		Start:   start,
		End:     end,
		Speaker: speaker,
		Text:    text,
		Action:  model.ActionKeep,
	}, nil
}

// parseTimingLine extracts start and end durations from a "start --> end" line.
func parseTimingLine(line string) (time.Duration, time.Duration, error) {
	parts := strings.Split(line, "-->")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("malformed timing line: %q", line)
	}

	startStr := strings.TrimSpace(parts[0])
	endField := strings.TrimSpace(parts[1])

	// WebVTT timing lines may include cue settings after end timestamp (e.g. line:0% position:50%)
	endFields := strings.Fields(endField)
	if len(endFields) == 0 {
		return 0, 0, fmt.Errorf("missing end timestamp: %q", line)
	}
	endStr := endFields[0]

	start, err := ParseTimestamp(startStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start timestamp %q: %w", startStr, err)
	}

	end, err := ParseTimestamp(endStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end timestamp %q: %w", endStr, err)
	}

	return start, end, nil
}

// extractSpeakerAndText parses speaker tags and strips formatting tags from cue lines.
func extractSpeakerAndText(lines []string) (string, string) {
	combined := strings.Join(lines, " ")
	combined = strings.TrimSpace(combined)

	if matches := voiceTagRe.FindStringSubmatch(combined); len(matches) == 3 {
		speaker := strings.TrimSpace(matches[1])
		text := tagRegex.ReplaceAllString(matches[2], "")
		return speaker, strings.TrimSpace(text)
	}

	if matches := speakerColon.FindStringSubmatch(combined); len(matches) == 3 {
		speaker := strings.TrimSpace(matches[1])
		text := tagRegex.ReplaceAllString(matches[2], "")
		return speaker, strings.TrimSpace(text)
	}

	cleanText := tagRegex.ReplaceAllString(combined, "")
	return "", strings.TrimSpace(cleanText)
}
