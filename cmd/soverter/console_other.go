//go:build !windows

package main

import (
	"fmt"
	"os"
)

func attachConsole() {}

func showError(msg string) { fmt.Fprintln(os.Stderr, msg) }

func fatal(msg string) {
	showError(msg)
	os.Exit(1)
}
