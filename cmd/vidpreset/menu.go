package main

import (
	"errors"
	"os"
	"path/filepath"

	"vidpreset/internal/preset"
	"vidpreset/internal/shellext"
	"vidpreset/internal/shellint"
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
	if c.Settings.Integration == "classic" {
		if err := shellint.Install(reg, exe, c.Presets, c.Settings.MenuMode); err != nil {
			return err
		}
		return shellext.Uninstall()
	}
	if err := shellext.Install(exe, c.Presets, c.Settings.MenuMode); err != nil {
		return err
	}
	return shellint.Uninstall(reg)
}

func uninstallMenu() error {
	return errors.Join(shellext.Uninstall(), shellint.Uninstall(shellint.WinRegistry{}))
}

func menuInstalled() bool { return shellext.Registered() || shellint.Installed() }
