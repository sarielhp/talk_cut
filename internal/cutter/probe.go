// Package cutter implements video probing, cut calculation, and FFmpeg execution.
package cutter

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// MediaInfo contains container and stream metadata extracted by ffprobe.
type MediaInfo struct {
	Duration   time.Duration `json:"duration"`
	Width      int           `json:"width"`
	Height     int           `json:"height"`
	VideoCodec string        `json:"video_codec"`
	AudioCodec string        `json:"audio_codec"`
	FormatName string        `json:"format_name"`
}

type ffprobeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
	Format struct {
		Duration   string `json:"duration"`
		FormatName string `json:"format_name"`
	} `json:"format"`
}

// ProbeMedia queries ffprobe for container and stream properties of a media file.
func ProbeMedia(ctx context.Context, filePath string) (MediaInfo, error) {
	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration,format_name",
		"-show_entries", "stream=codec_type,codec_name,width,height",
		"-of", "json",
		filePath,
	)

	out, err := cmd.Output()
	if err != nil {
		return MediaInfo{}, fmt.Errorf("running ffprobe on %q: %w", filePath, err)
	}

	var data ffprobeOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return MediaInfo{}, fmt.Errorf("parsing ffprobe json for %q: %w", filePath, err)
	}

	return parseMediaInfo(data)
}

// parseMediaInfo maps raw ffprobe JSON into a normalized MediaInfo structure.
func parseMediaInfo(data ffprobeOutput) (MediaInfo, error) {
	info := MediaInfo{
		FormatName: data.Format.FormatName,
	}

	if data.Format.Duration != "" {
		sec, err := strconv.ParseFloat(data.Format.Duration, 64)
		if err == nil {
			info.Duration = time.Duration(sec * float64(time.Second))
		}
	}

	for _, s := range data.Streams {
		if s.CodecType == "video" && info.VideoCodec == "" {
			info.VideoCodec = s.CodecName
			info.Width = s.Width
			info.Height = s.Height
		}
		if s.CodecType == "audio" && info.AudioCodec == "" {
			info.AudioCodec = s.CodecName
		}
	}

	return info, nil
}
