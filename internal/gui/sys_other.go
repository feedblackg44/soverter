//go:build !windows

package gui

import (
	"os/exec"
	"path/filepath"
)

func systemLight() bool { return false }

// PickVideoFile is only implemented on Windows.
func PickVideoFile(title string) (string, bool) { return "", false }

// ShowInFolder opens the containing folder of path.
func ShowInFolder(path string) { _ = exec.Command("xdg-open", filepath.Dir(path)).Start() }
