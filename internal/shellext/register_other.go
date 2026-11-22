//go:build !windows

package shellext

import (
	"errors"
	"vidpreset/internal/i18n"

	"vidpreset/internal/preset"
)

var errWin = i18n.Err("только Windows 11")

func Registered() bool                                      { return false }
func CertTrusted() bool                                     { return false }
func TrustCert() error                                      { return errWin }
func UntrustCert() error                                    { return errWin }
func Install(string, []preset.Preset, string, string) error { return errWin }
func Uninstall() error                                      { return errWin }
