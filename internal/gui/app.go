package gui

import (
	"os"
	"sync"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"vidpreset/internal/gui/theme"
)

// View is the content of a window.
type View interface {
	Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions
}

// Closer is implemented by views that want to veto closing (unsaved changes).
type Closer interface{ CanClose() bool }

// Win is a native window running a View.
type Win struct {
	W     *app.Window
	force atomic.Bool
}

var (
	themeOnce sync.Once
	theTheme  *theme.Theme
	openWins  atomic.Int32
)

// Theme returns the shared theme (dark unless Windows is set to light mode).
func Theme() *theme.Theme {
	themeOnce.Do(func() {
		p := theme.DarkPalette()
		if systemLight() {
			p = theme.LightPalette()
		}
		theTheme = theme.New(p)
	})
	return theTheme
}

// Main runs the GUI event loop; it must be called from the main goroutine.
func Main() { app.Main() }

// Open creates a window; the process exits when the last window is closed.
func Open(title string, w, h int, v View) *Win {
	win := &Win{W: new(app.Window)}
	win.W.Option(app.Title(title), app.Size(unit.Dp(w), unit.Dp(h)), app.MinSize(unit.Dp(420), unit.Dp(300)))
	openWins.Add(1)
	go win.loop(v)
	return win
}

// Invalidate requests a redraw from any goroutine.
func (w *Win) Invalidate() {
	if w != nil {
		w.W.Invalidate()
	}
}

// SetTitle changes the window title.
func (w *Win) SetTitle(s string) {
	if w != nil {
		w.W.Option(app.Title(s))
	}
}

// Close closes the window without asking the view.
func (w *Win) Close() {
	w.force.Store(true)
	w.W.Perform(system.ActionClose)
}

func (w *Win) loop(v View) {
	th := Theme()
	var ops op.Ops
	for {
		switch e := w.W.Event().(type) {
		case app.DestroyEvent:
			if openWins.Add(-1) <= 0 {
				os.Exit(0)
			}
			return
		case *app.ClosingEvent:
			if c, ok := v.(Closer); ok && !w.force.Load() && !c.CanClose() {
				e.Abort()
				w.W.Invalidate()
			}
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			paint.FillShape(gtx.Ops, th.P.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
			v.Layout(gtx, th)
			e.Frame(gtx.Ops)
		}
	}
}
