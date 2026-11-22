//go:build !windows

package shellext

import (
	"errors"

	"vidpreset/internal/preset"
)

var errWin = errors.New("только Windows 11")

func Registered() bool                              { return false }
func CertTrusted() bool                             { return false }
func TrustCert() error                              { return errWin }
func UntrustCert() error                            { return errWin }
func Install(string, []preset.Preset, string) error { return errWin }
func Uninstall() error                              { return errWin }
