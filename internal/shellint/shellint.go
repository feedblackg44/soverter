// Package shellint registers Explorer context-menu entries.
// The key layout is computed by Plan (pure) and applied through a Registry interface,
// so tests can use a mock instead of the real Windows registry.
//
// Entries are registered per file extension (SystemFileAssociations\.ext), because the
// perceived-type branch (SystemFileAssociations\video) is not honored on every system.
package shellint

import (
	"fmt"
	"strings"
	"vidpreset/internal/i18n"

	"vidpreset/internal/preset"
)

const classes = `Software\Classes\SystemFileAssociations`

// Extensions are the video file types that get the menu.
var Extensions = []string{"mp4", "mkv", "mov", "avi", "webm", "m4v", "ts", "wmv", "flv", "mpg", "mpeg", "3gp"}

// LegacyRoot is the perceived-type key used by earlier versions; it is only cleaned up.
const LegacyRoot = classes + `\video\shell\vidpreset`

// RootFor is the HKCU-relative key owned by vidpreset for one extension.
func RootFor(ext string) string { return classes + `\.` + ext + `\shell\vidpreset` }

// Roots lists every root key vidpreset owns.
func Roots() []string {
	r := make([]string, 0, len(Extensions)+1)
	for _, e := range Extensions {
		r = append(r, RootFor(e))
	}
	return append(r, LegacyRoot)
}

// Registry is the minimal HKCU registry surface we need.
type Registry interface {
	// SetString creates the key (and parents) if needed and sets value `name` ("" = default).
	SetString(key, name, value string) error
	// DeleteTree removes key and all subkeys; a missing key is not an error.
	DeleteTree(key string) error
	// DeleteIfEmpty removes key only if it has no subkeys and no values.
	DeleteIfEmpty(key string) error
}

// Entry is one registry value write.
type Entry struct{ Key, Name, Value string }

func planRoot(root, exe string, presets []preset.Preset, mode string) []Entry {
	q := `"` + exe + `"`
	es := []Entry{{root, "MUIVerb", i18n.T("Видео пресеты")}}
	if mode == "single" {
		return append(es, Entry{root + `\command`, "", q + ` pick "%1"`})
	}
	es = append(es, Entry{root, "SubCommands", ""})
	for i, p := range preset.Enabled(presets) {
		// Explorer sorts submenu items by key name; the numeric prefix keeps editor order.
		k := fmt.Sprintf(`%s\shell\%03d-%s`, root, i+1, p.ID)
		es = append(es,
			Entry{k, "MUIVerb", p.Name},
			Entry{k + `\command`, "", q + " run " + p.ID + ` "%1"`},
		)
	}
	return es
}

// Plan returns the registry writes for the given presets.
// mode is "submenu" (default) or "single".
func Plan(exe string, presets []preset.Preset, mode string) []Entry {
	var es []Entry
	for _, e := range Extensions {
		es = append(es, planRoot(RootFor(e), exe, presets, mode)...)
	}
	return es
}

// Install recreates the menu from scratch (idempotent).
func Install(r Registry, exe string, presets []preset.Preset, mode string) error {
	if err := Uninstall(r); err != nil {
		return err
	}
	for _, e := range Plan(exe, presets, mode) {
		if err := r.SetString(e.Key, e.Name, e.Value); err != nil {
			return fmt.Errorf("%s: %w", e.Key, err)
		}
	}
	return nil
}

// Uninstall removes everything Install created (and the legacy key),
// then prunes extension keys that became empty.
func Uninstall(r Registry) error {
	for _, root := range Roots() {
		if err := r.DeleteTree(root); err != nil {
			return err
		}
		// root = ...\.ext\shell\vidpreset -> try to prune ...\shell and ...\.ext
		shell := root[:strings.LastIndex(root, `\`)]
		ext := shell[:strings.LastIndex(shell, `\`)]
		for _, k := range []string{shell, ext} {
			if strings.HasSuffix(k, `\video`) || k == classes {
				continue // never prune shared perceived-type keys
			}
			if err := r.DeleteIfEmpty(k); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsSafeRoot guards DeleteTree implementations against deleting anything but our keys.
func IsSafeRoot(key string) bool {
	for _, r := range Roots() {
		if key == r || strings.HasPrefix(key, r+`\`) {
			return true
		}
	}
	return false
}

// IsSafePrune guards DeleteIfEmpty: only the shell/ext parents of our roots.
func IsSafePrune(key string) bool {
	for _, r := range Roots() {
		shell := r[:strings.LastIndex(r, `\`)]
		ext := shell[:strings.LastIndex(shell, `\`)]
		if key == shell || (key == ext && r != LegacyRoot) { // never the shared "video" key
			return true
		}
	}
	return false
}
