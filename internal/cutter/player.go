// Package cutter implements video probing, cut calculation, and FFmpeg execution.
package cutter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// VideoPlayer defines an external media player executable and its argument generator.
type VideoPlayer struct {
	Name      string
	ExePath   string
	BuildArgs func(start time.Duration, videoPath string) []string
}

// playerCandidate associates a player name with an argument builder.
type playerCandidate struct {
	name      string
	buildArgs func(start time.Duration, videoPath string) []string
}

var supportedPlayers = []playerCandidate{
	{
		name: "mpv",
		buildArgs: func(start time.Duration, videoPath string) []string {
			return []string{
				fmt.Sprintf("--start=%.2f", start.Seconds()),
				"--title=talk_cut preview - " + filepath.Base(videoPath),
				videoPath,
			}
		},
	},
	{
		name: "vlc",
		buildArgs: func(start time.Duration, videoPath string) []string {
			return []string{
				fmt.Sprintf("--start-time=%.2f", start.Seconds()),
				videoPath,
			}
		},
	},
	{
		name: "ffplay",
		buildArgs: func(start time.Duration, videoPath string) []string {
			return []string{
				"-ss", fmt.Sprintf("%.2f", start.Seconds()),
				"-window_title", "talk_cut preview - " + filepath.Base(videoPath),
				"-autoexit",
				videoPath,
			}
		},
	},
	{
		name: "totem",
		buildArgs: func(start time.Duration, videoPath string) []string {
			return []string{
				fmt.Sprintf("--seek=%d", int(start.Seconds()*1000)),
				videoPath,
			}
		},
	},
}

// DetectPlayer discovers the best available media player on the host system.
func DetectPlayer() (VideoPlayer, error) {
	if custom := os.Getenv("TALK_CUT_PLAYER"); custom != "" {
		if path, err := exec.LookPath(custom); err == nil {
			for _, p := range supportedPlayers {
				if p.name == custom {
					return VideoPlayer{Name: p.name, ExePath: path, BuildArgs: p.buildArgs}, nil
				}
			}
			return VideoPlayer{
				Name:    custom,
				ExePath: path,
				BuildArgs: func(_ time.Duration, vPath string) []string {
					return []string{vPath}
				},
			}, nil
		}
	}

	for _, p := range supportedPlayers {
		if path, err := exec.LookPath(p.name); err == nil {
			return VideoPlayer{Name: p.name, ExePath: path, BuildArgs: p.buildArgs}, nil
		}
	}

	return VideoPlayer{}, fmt.Errorf("no external video player found (install mpv or vlc)")
}

// LaunchPreview spawns an external player in the background at the specified start timestamp.
func LaunchPreview(player VideoPlayer, start time.Duration, videoPath string) (*exec.Cmd, error) {
	absPath, err := filepath.Abs(videoPath)
	if err != nil {
		absPath = videoPath
	}

	args := player.BuildArgs(start, absPath)
	cmd := exec.Command(player.ExePath, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching %s: %w", player.Name, err)
	}

	go func() {
		_ = cmd.Wait()
	}()

	return cmd, nil
}

// KillPreview terminates a running video player subprocess if active.
func KillPreview(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
