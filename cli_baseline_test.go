package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"
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
		// The help is clihelp's now rather than four hand-maintained blocks, so
		// the wording moved. What is asserted is that each page still says what
		// it is for and still lists the flags it documents.
		{name: "bare invocation prints help", args: nil, exit: 0,
			contains: []string{"Usage:", "talk_cut", "Commands:"}},
		{name: "--help", args: []string{"--help"}, exit: 0,
			contains: []string{"Interactive talk trimmer", "--meta-only", "--upload", "--update-youtube"}},
		{name: "-h", args: []string{"-h"}, exit: 0, contains: []string{"Usage:"}},
		{name: "--version", args: []string{"--version"}, exit: 0, contains: []string{"talk_cut v"}},
		{name: "-v", args: []string{"-v"}, exit: 0, contains: []string{"talk_cut v"}},

		// The root takes a recording directory, so an unrecognised word is a
		// path, not a mistyped command.
		{name: "a word that is not a directory", args: []string{"nosuchcommand"}, exit: 1,
			contains: []string{"recording directory", "no such file"}},

		{name: "youtube alone lists its subcommands", args: []string{"youtube"}, exit: 0,
			contains: []string{"update", "playlist", "setup", "status"}},
		{name: "youtube playlist alone lists its own", args: []string{"youtube", "playlist"}, exit: 0,
			contains: []string{"list", "create", "set-default", "add", "remove"}},

		{name: "an unknown youtube subcommand", args: []string{"youtube", "nosuch"}, exit: 1,
			contains: []string{`unknown command "nosuch" for "youtube"`}},
		{name: "an unknown playlist subcommand", args: []string{"youtube", "playlist", "nosuch"}, exit: 1,
			contains: []string{"nosuch"}},

		// New, and the reason the conversion is worth doing: a near miss is
		// named rather than merely rejected.
		{name: "a mistyped subcommand is suggested", args: []string{"youtube", "playlis"}, exit: 1,
			contains: []string{`Did you mean "playlist"`}},
		{name: "a mistyped command at the root", args: []string{"youtub"}, exit: 1,
			contains: []string{"youtub"}},

		// This used to print stdlib flag's own usage — a second, different help
		// from the curated one, with different wording and order, listing flags
		// the curated help deliberately omitted. Now there is one help, and an
		// unknown flag is one line.
		{name: "an unknown flag", args: []string{"--nosuchflag"}, exit: 1,
			contains: []string{"unknown flag"}, absent: []string{"Usage of talk_cut"}},
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

// Audit is what clihelp offers in exchange for declaring the command tree: it
// walks it and checks that every example still parses against the commands and
// flags that exist. It is the thing four hand-maintained usage blocks could not
// do, and it belongs in the test suite rather than in somebody's memory.
func TestCommandTreeAudits(t *testing.T) {
	if err := clihelp.Audit(buildApp(&cliOptions{})); err != nil {
		t.Errorf("the command tree does not audit: %v", err)
	}
}

// The flags the old parser bound are all still bound, under the same names.
func TestEveryOldFlagSurvives(t *testing.T) {
	app := buildApp(&cliOptions{})
	var buf bytes.Buffer
	app.RenderGlobal(clihelp.Options{Writer: &buf, Width: 200})
	help := clihelp.StripANSI(buf.String())

	for _, flag := range []string{
		"--output", "--url", "--layout", "--no-ai", "--dry-run", "--meta-only",
		"--upload", "--re-detect", "--update-youtube", "--channel", "--playlist",
		"--key-file", "--model", "--version",
	} {
		if !strings.Contains(help, flag) {
			t.Errorf("%s is no longer offered by the root", flag)
		}
	}
	// The two aliases are now shown. They were bound before and documented
	// nowhere — three separate BoolVar calls writing to one target, with the
	// help listing only the first. One spec string declares all three, and the
	// help says so, which is the more honest answer: a reader can now see that
	// --update-info works.
	for _, alias := range []string{"--update-info", "--update-details"} {
		if !strings.Contains(help, alias) {
			t.Errorf("%s was bound before this conversion and should still be offered", alias)
		}
	}
}
