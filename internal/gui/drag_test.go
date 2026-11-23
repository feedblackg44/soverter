package gui

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"soverter/internal/gui/theme"
)

// frameRig lays the editor out repeatedly with pointer events fed through a real input router.
type frameRig struct {
	t   *testing.T
	e   *Editor
	r   input.Router
	ops op.Ops
	th  *theme.Theme
}

func (f *frameRig) frame() {
	f.ops.Reset()
	gtx := layout.Context{
		Ops: &f.ops, Source: f.r.Source(), Constraints: layout.Exact(image.Pt(1300, 800)),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	f.e.Layout(gtx, f.th)
	f.r.Frame(&f.ops)
}

func (f *frameRig) send(kind pointer.Kind, x, y float32) {
	f.r.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y)})
	f.frame()
	f.frame()
}

func TestDragReordersPresets(t *testing.T) {
	f := &frameRig{t: t, e: demo(t), th: theme.New(theme.DarkPalette())}
	f.frame()
	f.frame()
	if f.e.rowPitch == 0 {
		t.Fatal("rows were not laid out")
	}
	// find a point inside row 1 by probing with clicks
	var x, y float32 = 100, 0
	for y = 40; y < 700; y += 6 {
		f.send(pointer.Move, x, y)
		f.send(pointer.Press, x, y)
		f.send(pointer.Release, x, y)
		if f.e.sel == 1 {
			break
		}
	}
	if f.e.sel != 1 {
		t.Fatal("could not click the second row")
	}
	before := ids(f.e)
	// drag it two rows down
	f.send(pointer.Press, x, y)
	for dy := float32(10); dy <= float32(2*f.e.rowPitch)+4; dy += 10 {
		f.send(pointer.Move, x, y+dy)
	}
	if f.e.dragIdx != 1 {
		t.Fatalf("drag did not start (dragIdx %d)", f.e.dragIdx)
	}
	f.send(pointer.Release, x, y+float32(2*f.e.rowPitch)+4)
	after := ids(f.e)
	if after[3] != before[1] || after[1] != before[2] {
		t.Fatalf("before %v after %v", before, after)
	}
	if f.e.sel != 3 {
		t.Fatalf("selection must follow the moved preset, got %d", f.e.sel)
	}
}
