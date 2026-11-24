package gui

import (
	"image"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"soverter/internal/gui/theme"
)

// The "+" and duplicate buttons add a preset in the middle of a frame (after listInput, before the list is
// drawn). The list must cope with a preset that has no row state yet (it used to panic with index out of range).
func TestAddAndDuplicateMidFrame(t *testing.T) {
	e := demo(t)
	th := theme.New(theme.DarkPalette())
	var ops op.Ops
	var router input.Router
	newGtx := func() layout.Context {
		ops.Reset()
		return layout.Context{Source: router.Source(), Ops: &ops, Constraints: layout.Exact(image.Pt(1770, 1200)), Metric: unit.Metric{PxPerDp: 1.5, PxPerSp: 1.5}}
	}
	gtx := newGtx()
	e.Layout(gtx, th)

	n := len(e.cfg.Presets)
	for _, add := range []func(){e.addPreset, e.dupPreset} {
		gtx = newGtx()
		e.listInput(gtx)
		add()
		e.presetList(gtx, th) // must not panic
		n++
		if len(e.cfg.Presets) != n || len(e.rows) < n {
			t.Fatalf("presets=%d rows=%d, want %d", len(e.cfg.Presets), len(e.rows), n)
		}
	}
	e.Layout(newGtx(), th)
}
