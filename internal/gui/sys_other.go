//go:build !windows

package gui

import (
	"os/exec"
	"path/filepath"

	"gioui.org/io/event"

	"soverter/internal/gui/theme"
)

func systemLight() bool { return false }

// styleChrome tints the native title bar; only implemented on Windows.
func styleChrome(event.Event, theme.Palette) {}

// PickVideoFile is only implemented on Windows.
func PickVideoFile(title string) (string, bool) { return "", false }

// ShowInFolder opens the containing folder of path.
func ShowInFolder(path string) { _ = exec.Command("xdg-open", filepath.Dir(path)).Start() }
