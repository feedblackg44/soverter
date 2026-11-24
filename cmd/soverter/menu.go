package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"soverter/internal/encoders"
	"soverter/internal/preset"
	"soverter/internal/shellext"
	"soverter/internal/shellint"
)

// installMenu registers the context menu using the configured integration.
// "modern" = Windows 11 IExplorerCommand menu; "classic" = registry verbs (Windows 10 / "more options").
// Only after the chosen integration succeeds is the other one removed, so a failure never leaves no menu.
func installMenu(c *preset.Config) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.Abs(exe)
	reg := shellint.WinRegistry{}
	// The classic menu is the default; the Windows 11 one is opt-in, and a build without its package falls back to classic.
	if c.Settings.Integration != "modern" || !shellext.Available() {
		if err := shellint.Install(reg, exe, c.Presets, c.Settings.MenuMode, c.Settings.MenuName(), preset.InfoMenuLabel()); err != nil {
			return err
		}
		return shellext.Uninstall()
	}
	if err := shellext.Install(exe, c.Presets, c.Settings.MenuMode, c.Settings.MenuTitle, preset.InfoMenuLabel()); err != nil {
		return err
	}
	return shellint.Uninstall(reg)
}

func uninstallMenu() error {
	return errors.Join(shellext.Uninstall(), shellint.Uninstall(shellint.WinRegistry{}))
}

func menuInstalled() bool { return shellext.Registered() || shellint.Installed() }

// hardwareCheck returns a runner hook that explains why a hardware encoder cannot run on this machine.
// It uses the cached trial-encode report (computed once, then read from disk), so software presets stay instant.
func hardwareCheck(ffmpeg string) func(string) string {
	return func(name string) string {
		if !strings.HasSuffix(name, "_nvenc") && !strings.HasSuffix(name, "_amf") && !strings.HasSuffix(name, "_qsv") {
			return ""
		}
		rep, err := encoders.Probe(context.Background(), ffmpeg, false)
		if err != nil {
			return ""
		}
		return rep.Why(name)
	}
}
