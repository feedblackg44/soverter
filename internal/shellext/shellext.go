// Package shellext integrates with the Windows 11 context menu (IExplorerCommand).
//
// The menu is a small C++ COM DLL (see /shellext) shipped inside a signed MSIX package
// (built by shellext/package.ps1 and embedded here). The DLL reads %APPDATA%\soverter\menu.txt at menu
// time, so editing presets only rewrites that file; the package is registered once.
package shellext

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"embed"
	"encoding/hex"
	"strings"

	"soverter/internal/preset"
)

//go:embed assets
var assets embed.FS

const (
	PackageName = "soverter.ContextMenu"
	msixFile    = "assets/soverter.msix"
	cerFile     = "assets/soverter.cer"
)

func msixBytes() []byte {
	b, _ := assets.ReadFile(msixFile)
	return b
}

// certBytes returns the embedded public certificate (DER) of a self-signed build;
// nil when the package is signed with a real certificate that Windows already trusts.
func certBytes() []byte {
	b, err := assets.ReadFile(cerFile)
	if err != nil {
		return nil
	}
	return b
}

// CertThumbprint is the SHA-1 thumbprint (upper-case hex) of the embedded certificate, or "".
func CertThumbprint() string {
	der := certBytes()
	if der == nil {
		return ""
	}
	if _, err := x509.ParseCertificate(der); err != nil {
		return ""
	}
	s := sha1.Sum(der)
	return strings.ToUpper(hex.EncodeToString(s[:]))
}

// Version identifies the embedded package so a different build triggers re-registration.
func Version() string {
	h := sha256.Sum256(msixBytes())
	return hex.EncodeToString(h[:8])
}

// MenuFile renders menu.txt: exe, mode, the menu title and one "id<TAB>name" line per enabled preset.
func MenuFile(exe string, presets []preset.Preset, mode, title string) []byte {
	if mode != "single" {
		mode = "submenu"
	}
	var b strings.Builder
	b.WriteString("exe=" + exe + "\n")
	b.WriteString("mode=" + mode + "\n")
	title = strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(title))
	if title != "" {
		b.WriteString("title=" + title + "\n") // the DLL falls back to "soverter" without it
	}
	for _, p := range preset.Enabled(presets) {
		name := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(p.Name)
		b.WriteString(p.ID + "\t" + name + "\n")
	}
	return []byte(b.String())
}

// Available reports whether this build contains the Windows 11 menu package. The package is not committed to
// git: it is produced by shellext/package.ps1 before `go build` (see release.ps1).
func Available() bool { return len(msixBytes()) > 0 }
