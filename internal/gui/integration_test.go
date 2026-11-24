package gui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"soverter/internal/gui/theme"
	"soverter/internal/i18n"
	"soverter/internal/preset"
)

func TestIntegrationDefaultsToClassic(t *testing.T) {
	e := newEditor("", preset.Defaults(), EditorHooks{})
	e.settingsDialog()
	if e.setSt.integration != "classic" {
		t.Fatalf("default integration = %q, want classic", e.setSt.integration)
	}
	e.cfg.Settings.Integration = ""
	e.settingsDialog()
	if e.setSt.integration != "classic" {
		t.Fatalf("empty integration = %q, want classic", e.setSt.integration)
	}
	e.cfg.Settings.Integration = "modern"
	e.settingsDialog()
	if e.setSt.integration != "modern" {
		t.Fatalf("explicit modern = %q", e.setSt.integration)
	}
}

func TestIntegrationHint(t *testing.T) {
	i18n.SetLang("en")
	defer i18n.SetLang("")
	if h := integrationHint("modern", true); !strings.Contains(h, "Beta") || !strings.Contains(h, "UAC") {
		t.Fatalf("modern hint must mention Beta and UAC: %q", h)
	}
	if h := integrationHint("classic", true); strings.Contains(h, "UAC") {
		t.Fatalf("classic hint must not mention UAC: %q", h)
	}
}

func TestSettingsDialogRendersBothKinds(t *testing.T) {
	th := theme.New(theme.DarkPalette())
	for _, avail := range []bool{true, false} {
		e := newEditor("", preset.Defaults(), EditorHooks{ModernAvailable: func() bool { return avail }})
		e.cfg.Settings.Integration = "modern"
		e.settingsDialog()
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Constraints: layout.Exact(image.Pt(1770, 1400)), Metric: unit.Metric{PxPerDp: 1.5, PxPerSp: 1.5}}
		e.Layout(gtx, th)
	}
}
