package gui

import "vidpreset/internal/preset"

// DemoEditor builds an editor without a window (for headless rendering and tests).
func DemoEditor(cfg *preset.Config, sel int) View {
	e := newEditor("", cfg, EditorHooks{})
	e.sel = sel
	return e
}
