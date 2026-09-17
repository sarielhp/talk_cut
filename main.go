// Package main is the entry point for the zoomcut CLI tool.
package main

import (
	"fmt"
	"os"
)

// Version is the current semantic version of zoomcut.
const Version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "-v" || args[0] == "--version") {
		fmt.Printf("zoomcut v%s\n", Version)
		return nil
	}

	fmt.Printf("zoomcut v%s - Zoom Talk Trimmer & Publisher\n", Version)
	fmt.Println("Run with --help for usage instructions.")
	return nil
}
