package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The command-line surface had no test at all: 108 test functions in this
// repository and none reached parseCLIFlags or the dispatcher. This records what
// every path actually does, so that a change to the surface is a decision
// somebody made rather than something that happened.
//
// It builds the binary and runs it, because that is the only way to see what a
// user sees — the exit code, the stream each line goes to, and the help that
// stdlib's flag package prints when nothing intercepts it.
func TestCLISurface(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "talk_cut")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building: %v\n%s", err, out)
	}
	home := t.TempDir()

	for _, tt := range []struct {
		name     string
		args     []string
		exit     int
		contains []string
		absent   []string
	}{
		{name: "bare invocation prints help", args: nil, exit: 0,
			contains: []string{"talk_cut v", "Usage:", "Options:"}},
		{name: "--help", args: []string{"--help"}, exit: 0,
			contains: []string{"Interactive Talk Trimmer", "--meta-only", "--upload"}},
		{name: "-h", args: []string{"-h"}, exit: 0, contains: []string{"Usage:"}},
		{name: "--version", args: []string{"--version"}, exit: 0, contains: []string{"talk_cut v"}},
		{name: "-v", args: []string{"-v"}, exit: 0, contains: []string{"talk_cut v"}},

		// The root takes a recording directory, so an unrecognised word is a
		// path, not a mistyped command.
		{name: "a word that is not a directory", args: []string{"nosuchcommand"}, exit: 1,
			contains: []string{"recording directory", "no such file"}},

		{name: "youtube alone lists its subcommands", args: []string{"youtube"}, exit: 0,
			contains: []string{"youtube update", "youtube playlist", "youtube setup"}},
		{name: "youtube playlist alone lists its own", args: []string{"youtube", "playlist"}, exit: 0,
			contains: []string{"playlist list", "playlist create", "set-default"}},

		{name: "an unknown youtube subcommand", args: []string{"youtube", "nosuch"}, exit: 1,
			contains: []string{`unknown youtube command "nosuch"`}},
		{name: "an unknown playlist subcommand", args: []string{"youtube", "playlist", "nosuch"}, exit: 1,
			contains: []string{"nosuch"}},

		// stdlib's flag package prints its own usage here, which is a different
		// help from the curated one above — different wording, different order,
		// and it lists flags the curated help deliberately omits.
		{name: "an unknown flag", args: []string{"--nosuchflag"}, exit: 1,
			contains: []string{"not defined"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(bin, tt.args...)
			cmd.Env = append(os.Environ(), "HOME="+home)
			cmd.Stdin = strings.NewReader("")
			out, _ := cmd.CombinedOutput()
			body := string(out)

			got := cmd.ProcessState.ExitCode()
			if got != tt.exit {
				t.Errorf("exit = %d, want %d\n%s", got, tt.exit, body)
			}
			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Errorf("output is missing %q:\n%s", want, body)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(body, unwanted) {
					t.Errorf("output unexpectedly contains %q:\n%s", unwanted, body)
				}
			}
		})
	}
}
