// Package cutter implements video probing, cut calculation, and FFmpeg execution.
package cutter

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"talk_cut/internal/model"
)

// CutMode determines how video slices are processed.
type CutMode int

const (
	// ModeLossless performs stream copy (-c copy) for rapid slicing without re-encoding.
	ModeLossless CutMode = iota
	// ModeReencode performs exact frame cuts with H.264 re-encoding.
	ModeReencode
)

// CutProgress reports live status during an FFmpeg cut operation.
type CutProgress struct {
	Stage       string        `json:"stage"` // "probing", "slicing", "joining", "done"
	PartIndex   int           `json:"part_index"`
	TotalParts  int           `json:"total_parts"`
	Percent     float64       `json:"percent"`
	CurrentTime time.Duration `json:"current_time"`
}

// CutOptions specifies configuration for a video cutting job.
type CutOptions struct {
	InputPath  string
	OutputPath string
	Cuts       []model.CutInterval
	Mode       CutMode
	OnProgress func(CutProgress)
}

// CutVideo cuts and splices video according to the provided CutOptions.
func CutVideo(ctx context.Context, opts CutOptions) error {
	if opts.InputPath == "" || opts.OutputPath == "" {
		return fmt.Errorf("input and output paths must be specified")
	}

	info, err := ProbeMedia(ctx, opts.InputPath)
	if err != nil {
		return fmt.Errorf("probing input media: %w", err)
	}

	kept := GetKeptIntervals(info.Duration, opts.Cuts)
	if len(kept) == 0 {
		return fmt.Errorf("no video segments remain after cuts")
	}

	if len(kept) == 1 {
		return renderSingleSegment(ctx, opts, kept[0])
	}

	return renderMultiSegments(ctx, opts, kept)
}

// renderSingleSegment extracts a single kept segment directly to the output destination.
func renderSingleSegment(ctx context.Context, opts CutOptions, seg model.CutInterval) error {
	notifyProgress(opts, CutProgress{
		Stage:      "slicing",
		PartIndex:  1,
		TotalParts: 1,
		Percent:    10.0,
	})

	args := buildSliceArgs(opts.InputPath, opts.OutputPath, seg.Start, seg.End, opts.Mode)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	if err := runFFmpegWithProgress(cmd, seg.Duration(), opts.OnProgress); err != nil {
		return fmt.Errorf("rendering single segment: %w", err)
	}

	notifyProgress(opts, CutProgress{
		Stage:      "done",
		PartIndex:  1,
		TotalParts: 1,
		Percent:    100.0,
	})

	return nil
}

// renderMultiSegments slices multiple kept parts into a temp folder and concats them.
func renderMultiSegments(ctx context.Context, opts CutOptions, kept []model.CutInterval) error {
	tempDir, err := os.MkdirTemp("", "talk_cut_*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var partFiles []string
	ext := filepath.Ext(opts.OutputPath)
	if ext == "" {
		ext = ".mp4"
	}

	for i, seg := range kept {
		partName := filepath.Join(tempDir, fmt.Sprintf("part_%03d%s", i, ext))
		notifyProgress(opts, CutProgress{
			Stage:      "slicing",
			PartIndex:  i + 1,
			TotalParts: len(kept),
			Percent:    float64(i) / float64(len(kept)) * 80.0,
		})

		args := buildSliceArgs(opts.InputPath, partName, seg.Start, seg.End, opts.Mode)
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("slicing part %d (%s-%s): %w", i+1, seg.Start, seg.End, err)
		}
		partFiles = append(partFiles, partName)
	}

	return concatenateParts(ctx, opts, tempDir, partFiles, len(kept))
}

// concatenateParts writes the concat manifest and runs FFmpeg to merge part files.
func concatenateParts(ctx context.Context, opts CutOptions, tempDir string, parts []string, totalParts int) error {
	manifestPath := filepath.Join(tempDir, "concat.txt")
	manifestFile, err := os.Create(manifestPath)
	if err != nil {
		return fmt.Errorf("creating concat manifest: %w", err)
	}

	for _, p := range parts {
		abs, absErr := filepath.Abs(p)
		if absErr != nil {
			manifestFile.Close()
			return fmt.Errorf("resolving part path: %w", absErr)
		}
		// Escape single quotes for ffmpeg concat demuxer
		escaped := strings.ReplaceAll(abs, "'", "'\\''")
		if _, writeErr := fmt.Fprintf(manifestFile, "file '%s'\n", escaped); writeErr != nil {
			manifestFile.Close()
			return fmt.Errorf("writing concat manifest: %w", writeErr)
		}
	}
	manifestFile.Close()

	notifyProgress(opts, CutProgress{
		Stage:      "joining",
		PartIndex:  totalParts,
		TotalParts: totalParts,
		Percent:    85.0,
	})

	concatArgs := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", manifestPath,
		"-c", "copy",
		opts.OutputPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", concatArgs...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("concatenating parts: %w", err)
	}

	notifyProgress(opts, CutProgress{
		Stage:      "done",
		PartIndex:  totalParts,
		TotalParts: totalParts,
		Percent:    100.0,
	})

	return nil
}

// buildSliceArgs constructs the ffmpeg CLI arguments for a slice cut.
func buildSliceArgs(input, output string, start, end time.Duration, mode CutMode) []string {
	duration := end - start
	startSec := fmt.Sprintf("%.3f", start.Seconds())
	durSec := fmt.Sprintf("%.3f", duration.Seconds())

	baseArgs := []string{
		"-y",
		"-ss", startSec,
		"-t", durSec,
		"-i", input,
	}

	if mode == ModeLossless {
		return append(baseArgs, "-c", "copy", "-avoid_negative_ts", "make_zero", output)
	}

	return append(baseArgs,
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "20",
		"-c:a", "aac",
		"-b:a", "192k",
		output,
	)
}

// runFFmpegWithProgress executes ffmpeg and parses progress lines from stdout/stderr.
func runFFmpegWithProgress(cmd *exec.Cmd, totalDuration time.Duration, onProgress func(CutProgress)) error {
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return cmd.Run()
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if onProgress != nil && strings.Contains(line, "time=") {
			curTime := parseProgressTime(line)
			percent := 0.0
			if totalDuration > 0 {
				percent = (float64(curTime) / float64(totalDuration)) * 100.0
			}
			onProgress(CutProgress{
				Stage:       "slicing",
				PartIndex:   1,
				TotalParts:  1,
				Percent:     percent,
				CurrentTime: curTime,
			})
		}
	}

	return cmd.Wait()
}

// parseProgressTime parses "time=00:01:23.45" from ffmpeg stderr lines.
func parseProgressTime(line string) time.Duration {
	idx := strings.Index(line, "time=")
	if idx == -1 {
		return 0
	}
	part := line[idx+5:]
	fields := strings.Fields(part)
	if len(fields) == 0 {
		return 0
	}
	timeStr := fields[0]
	timeParts := strings.Split(timeStr, ":")
	if len(timeParts) != 3 {
		return 0
	}

	h, _ := strconv.ParseFloat(timeParts[0], 64)
	m, _ := strconv.ParseFloat(timeParts[1], 64)
	s, _ := strconv.ParseFloat(timeParts[2], 64)

	return time.Duration((h*3600 + m*60 + s) * float64(time.Second))
}

// notifyProgress safely calls the progress callback if non-nil.
func notifyProgress(opts CutOptions, p CutProgress) {
	if opts.OnProgress != nil {
		opts.OnProgress(p)
	}
}
