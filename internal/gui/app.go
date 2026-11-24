package gui

import (
	"os"
	"sync"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"soverter/internal/gui/theme"
	"soverter/internal/i18n"
	"soverter/internal/preset"
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
	// OnScale, if set, is called when the user changes the scale with the keyboard in this window.
	OnScale func(percent int)
}

var (
	themeOnce sync.Once
	theTheme  *theme.Theme
	openWins  atomic.Int32

	winMu sync.Mutex
	wins  = map[*Win]struct{}{}

	uiScale atomic.Int32 // interface scale in percent
)

func init() { uiScale.Store(100) }

// Scale steps used by Ctrl+= / Ctrl+-.
var scaleSteps = []int{75, 90, 100, 110, 125, 150, 175, 200, 250}

// Configure applies the language and scale from the settings (call before opening windows).
func Configure(s preset.Settings) {
	i18n.SetLang(s.Language)
	if s.UIScale > 0 {
		uiScale.Store(int32(clampScale(s.UIScale)))
	}
}

func clampScale(p int) int {
	if p < 50 {
		return 50
	}
	if p > 300 {
		return 300
	}
	return p
}

// Scale returns the interface scale in percent.
func Scale() int { return int(uiScale.Load()) }

// SetScale changes the interface scale of all windows.
func SetScale(p int) {
	uiScale.Store(int32(clampScale(p)))
	InvalidateAll()
}

// stepScale moves to the next/previous entry of scaleSteps and returns the new percent.
func stepScale(dir int) int {
	cur := Scale()
	idx := 0
	for i, s := range scaleSteps {
		if s <= cur {
			idx = i
		}
	}
	if scaleSteps[idx] < cur {
		// off-grid value: snap to the neighbouring grid point in the requested direction
		if dir > 0 {
			idx++
		}
	} else {
		idx += dir
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(scaleSteps) {
		idx = len(scaleSteps) - 1
	}
	return scaleSteps[idx]
}

// InvalidateAll redraws every open window.
func InvalidateAll() {
	winMu.Lock()
	defer winMu.Unlock()
	for w := range wins {
		w.W.Invalidate()
	}
}

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
	s := float32(Scale()) / 100
	win.W.Option(app.Title(title), app.Size(unit.Dp(float32(w)*s), unit.Dp(float32(h)*s)), app.MinSize(unit.Dp(420), unit.Dp(300)))
	openWins.Add(1)
	winMu.Lock()
	wins[win] = struct{}{}
	winMu.Unlock()
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
			winMu.Lock()
			delete(wins, w)
			winMu.Unlock()
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
			s := float32(Scale()) / 100
			gtx.Metric.PxPerDp *= s
			gtx.Metric.PxPerSp *= s
			w.scaleKeys(gtx)
			paint.FillShape(gtx.Ops, th.P.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
			v.Layout(gtx, th)
			e.Frame(gtx.Ops)
		default:
			styleChrome(e, th.P) // the native window handle arrives as a platform event
		}
	}
}

// scaleKeys handles Ctrl+= / Ctrl+- / Ctrl+0.
func (w *Win) scaleKeys(gtx layout.Context) {
	mods := key.ModShortcut
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "=", Required: mods, Optional: key.ModShift},
			key.Filter{Name: "+", Required: mods, Optional: key.ModShift},
			key.Filter{Name: "-", Required: mods},
			key.Filter{Name: "0", Required: mods},
		)
		if !ok {
			return
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		n := Scale()
		switch e.Name {
		case "=", "+":
			n = stepScale(1)
		case "-":
			n = stepScale(-1)
		case "0":
			n = 100
		}
		SetScale(n)
		if w.OnScale != nil {
			w.OnScale(n)
		}
	}
}
