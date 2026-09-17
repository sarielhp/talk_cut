// Package main is the entry point for the talk_cut CLI tool.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
)

//go:embed VERSION
var rawVersion string

// Version is the current semantic version of talk_cut.
var Version = strings.TrimSpace(rawVersion)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "-v" || args[0] == "--version") {
		fmt.Printf("talk_cut v%s\n", Version)
		return nil
	}

	fmt.Printf("talk_cut v%s - Talk Trimmer & Publisher\n", Version)
	fmt.Println("Run with --help for usage instructions.")
	return nil
}
