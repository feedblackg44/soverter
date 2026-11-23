package gui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"soverter/internal/gui/theme"
)

// Segoe Fluent Icons code points used by the UI.
const (
	IcAdd      = ''
	IcDelete   = ''
	IcCopy     = ''
	IcUp       = ''
	IcDown     = ''
	IcSettings = ''
	IcSave     = ''
	IcPlay     = ''
	IcUndo     = ''
	IcImport   = ''
	IcSearch   = ''
	IcCheck    = ''
	IcMenu     = ''
	IcWarning  = ''
	IcError    = ''
	IcFolder   = ''
)

// ---- wrapping flow -------------------------------------------------------------------------------------------

// Flow lays out n items left to right and wraps them onto new lines.
func Flow(gtx layout.Context, n int, gap unit.Dp, item func(gtx layout.Context, i int) layout.Dimensions) layout.Dimensions {
	g := gtx.Dp(gap)
	maxX := gtx.Constraints.Max.X
	type placed struct {
		call op.CallOp
		pos  image.Point
	}
	var items []placed
	x, y, rowH := 0, 0, 0
	width := 0
	for i := 0; i < n; i++ {
		rec := op.Record(gtx.Ops)
		c := gtx
		c.Constraints.Min = image.Point{}
		d := item(c, i)
		call := rec.Stop()
		if x > 0 && x+d.Size.X > maxX {
			x = 0
			y += rowH + g
			rowH = 0
		}
		items = append(items, placed{call, image.Pt(x, y)})
		x += d.Size.X + g
		if d.Size.Y > rowH {
			rowH = d.Size.Y
		}
		if x-g > width {
			width = x - g
		}
	}
	for _, it := range items {
		st := op.Offset(it.pos).Push(gtx.Ops)
		it.call.Add(gtx.Ops)
		st.Pop()
	}
	return layout.Dimensions{Size: image.Pt(width, y+rowH)}
}

// ---- segmented choice ----------------------------------------------------------------------------------------

// Opt is one choice of a segmented control.
type Opt struct {
	Val, Label string
	Off        bool // disabled
}

// Seg is a wrapping group of pill-shaped choices; one click selects.
type Seg struct {
	btns []widget.Clickable
	// Hit is true after Layout if an option was clicked this frame (needed when the clicked value may be "").
	Hit bool
}

// Layout draws the options and returns the value that was clicked this frame ("" if none).
func (s *Seg) Layout(gtx layout.Context, th *theme.Theme, opts []Opt, sel string) (string, layout.Dimensions) {
	for len(s.btns) < len(opts) {
		s.btns = append(s.btns, widget.Clickable{})
	}
	p := th.P
	clicked := ""
	s.Hit = false
	dims := Flow(gtx, len(opts), 8, func(gtx layout.Context, i int) layout.Dimensions {
		o := opts[i]
		b := &s.btns[i]
		if b.Clicked(gtx) && !o.Off {
			clicked = o.Val
			s.Hit = true
		}
		return b.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			on := o.Val == sel
			bg, border, fg := p.Surface, p.Border, p.TextDim
			switch {
			case on:
				bg, border, fg = p.AccentWash, p.Accent, p.Text
			case o.Off:
				fg = p.TextMuted
			case b.Hovered():
				bg, border, fg = p.Raised, p.BorderStrong, p.Text
			}
			return theme.Box(gtx, bg, border, 8, layout.Inset{Top: 9, Bottom: 9, Left: 14, Right: 14}, func(gtx layout.Context) layout.Dimensions {
				return th.Text(gtx, 14, font.Medium, fg, o.Label)
			})
		})
	})
	return clicked, dims
}

// ---- text field ----------------------------------------------------------------------------------------------

// Field is a single- or multi-line text input with a border and hint.
type Field struct {
	ed    widget.Editor
	click widget.Clickable
	last  string
	Mono  bool
	Rows  int // visible rows for multiline fields (0 = single line)
}

// NewField creates a single-line field.
func NewField() *Field {
	f := &Field{}
	f.ed.SingleLine = true
	return f
}

// NewArea creates a multi-line field with the given number of visible rows.
func NewArea(rows int) *Field { return &Field{Rows: rows} }

// Set replaces the text without reporting it as a user change.
func (f *Field) Set(s string) {
	if f.ed.Text() == s {
		f.last = s
		return
	}
	f.ed.SetText(s)
	f.last = s
}

// Text returns the current text.
func (f *Field) Text() string { return f.ed.Text() }

// Changed reports whether the user edited the text since the last call.
func (f *Field) Changed() bool {
	if t := f.ed.Text(); t != f.last {
		f.last = t
		return true
	}
	return false
}

// Focus gives the field keyboard focus.
func (f *Field) Focus(gtx layout.Context) { gtx.Execute(key.FocusCmd{Tag: &f.ed}) }

// Layout draws the field.
func (f *Field) Layout(gtx layout.Context, th *theme.Theme, hint string) layout.Dimensions {
	p := th.P
	if f.click.Clicked(gtx) {
		f.Focus(gtx)
	}
	focused := gtx.Focused(&f.ed)
	border := p.Border
	if focused {
		border = p.Accent
	}
	fnt := th.Font(font.Normal)
	size := float32(15)
	if f.Mono {
		fnt = font.Font{Typeface: th.Mono}
		size = 14
	}
	return f.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := p.Bg
		return theme.Box(gtx, bg, border, 8, layout.Inset{Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			align := layout.W
			pad := layout.Inset{}
			if f.Rows > 0 {
				align = layout.NW
				pad = layout.Inset{Top: 11, Bottom: 11}
				gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(float32(f.Rows)*22 + 22))
			} else {
				gtx.Constraints.Min.Y = gtx.Dp(42)
			}
			return pad.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// monospace fonts sit high in their line box: nudge them down so the text looks centered
				if f.Mono {
					defer op.Offset(image.Pt(0, gtx.Sp(unit.Sp(size*monoNudge)))).Push(gtx.Ops).Pop()
				}
				return layout.Stack{Alignment: align}.Layout(gtx,
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return f.ed.Layout(gtx, th.Shaper, fnt, unit.Sp(size), theme.ColorOp(gtx, p.Text), theme.ColorOp(gtx, p.AccentWash))
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						if f.ed.Len() != 0 || hint == "" {
							return layout.Dimensions{}
						}
						return widget.Label{MaxLines: 1, Truncator: "…"}.Layout(gtx, th.Shaper, fnt, unit.Sp(size), hint, theme.ColorOp(gtx, p.TextMuted))
					}),
				)
			})
		})
	})
}

// ---- small controls ------------------------------------------------------------------------------------------

// IconBtn is a square icon-only button.
type IconBtn struct{ widget.Clickable }

// Layout draws the glyph; danger tints it on hover.
func (b *IconBtn) Layout(gtx layout.Context, th *theme.Theme, icon rune, danger bool) layout.Dimensions {
	p := th.P
	return b.Clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		fg := p.TextDim
		bg := theme.Alpha(p.Surface, 0)
		if b.Hovered() {
			bg, fg = p.Raised, p.Text
			if danger {
				fg = p.Danger
			}
		}
		if !gtx.Enabled() {
			fg = p.TextMuted
		}
		s := gtx.Dp(32)
		sz := image.Pt(s, s)
		theme.Fill(gtx, bg, sz, 8)
		st := op.Offset(image.Pt(s/4, s/5)).Push(gtx.Ops)
		th.Icon(gtx, icon, 16, fg)
		st.Pop()
		return layout.Dimensions{Size: sz}
	})
}

// Switch is an on/off toggle.
type Switch struct{ widget.Bool }

// Layout draws the toggle; the Value field holds the state.
func (s *Switch) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	p := th.P
	return s.Bool.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w, h := gtx.Dp(34), gtx.Dp(20)
		bg := p.BorderStrong
		if s.Value {
			bg = p.Accent
		}
		theme.Fill(gtx, bg, image.Pt(w, h), 10)
		d := h - gtx.Dp(6)
		x := gtx.Dp(3)
		if s.Value {
			x = w - gtx.Dp(3) - d
		}
		knob := p.Text
		if s.Value {
			knob = p.OnAccent
		}
		paint.FillShape(gtx.Ops, knob, clip.Ellipse{Min: image.Pt(x, gtx.Dp(3)), Max: image.Pt(x+d, gtx.Dp(3)+d)}.Op(gtx.Ops))
		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}

// Slider is a horizontal 0..max integer slider.
type Slider struct{ f widget.Float }

// Layout draws the slider; v is the current value, the new value and whether it changed are returned.
func (s *Slider) Layout(gtx layout.Context, th *theme.Theme, v, max int) (int, bool, layout.Dimensions) {
	p := th.P
	h := gtx.Dp(24)
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, h)
	if !s.f.Dragging() {
		s.f.Value = float32(v) / float32(max)
	}
	changed := s.f.Update(gtx)
	nv := v
	if changed {
		nv = int(s.f.Value*float32(max) + 0.5)
	}
	d := s.f.Layout(gtx, layout.Horizontal, 8)
	w := d.Size.X
	trackH := gtx.Dp(4)
	ty := (h - trackH) / 2
	r := gtx.Dp(8)
	pos := int(float32(w-2*r)*float32(nv)/float32(max)) + r
	st := op.Offset(image.Pt(0, ty)).Push(gtx.Ops)
	theme.Fill(gtx, p.Raised, image.Pt(w, trackH), 2)
	paint.FillShape(gtx.Ops, p.Accent, clip.UniformRRect(image.Rect(0, 0, pos, trackH), trackH/2).Op(gtx.Ops))
	st.Pop()
	paint.FillShape(gtx.Ops, p.Accent, clip.Ellipse{Min: image.Pt(pos-r, h/2-r), Max: image.Pt(pos+r, h/2+r)}.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, p.Surface, clip.Ellipse{Min: image.Pt(pos-r+gtx.Dp(3), h/2-r+gtx.Dp(3)), Max: image.Pt(pos+r-gtx.Dp(3), h/2+r-gtx.Dp(3))}.Op(gtx.Ops))
	return nv, changed, layout.Dimensions{Size: image.Pt(w, h)}
}

// ---- layout helpers ------------------------------------------------------------------------------------------

// Vert lays out children top to bottom with a gap between them.
func Vert(gtx layout.Context, gap unit.Dp, children ...layout.Widget) layout.Dimensions {
	var fc []layout.FlexChild
	for i, c := range children {
		c := c
		if i > 0 {
			fc = append(fc, layout.Rigid(Spacer(gap)))
		}
		fc = append(fc, layout.Rigid(c))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, fc...)
}

// Labeled puts a small caption above a control.
func Labeled(th *theme.Theme, caption string, body layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return Vert(gtx, 6,
			func(gtx layout.Context) layout.Dimensions {
				return th.Text(gtx, 13, font.Medium, th.P.TextDim, caption)
			},
			body)
	}
}

// Hint is dim wrapping helper text.
func Hint(th *theme.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return th.Paragraph(gtx, 13, th.P.TextMuted, s) }
}

// Section is a titled card.
func Section(gtx layout.Context, th *theme.Theme, title, sub string, body layout.Widget) layout.Dimensions {
	p := th.P
	return theme.Box(gtx, p.Surface, p.Border, 14, layout.UniformInset(18), func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return Vert(gtx, 14,
			func(gtx layout.Context) layout.Dimensions {
				return Vert(gtx, 3,
					func(gtx layout.Context) layout.Dimensions { return th.Text(gtx, 15, font.Medium, p.Text, title) },
					func(gtx layout.Context) layout.Dimensions {
						if sub == "" {
							return layout.Dimensions{}
						}
						return th.Paragraph(gtx, 12, p.TextMuted, sub)
					})
			},
			body)
	})
}

// Banner is a tinted message strip (kind: "error", "warning" or "info").
func Banner(gtx layout.Context, th *theme.Theme, kind, msg string) layout.Dimensions {
	p := th.P
	c := p.Accent
	ic := IcCheck
	switch kind {
	case "error":
		c, ic = p.Danger, IcError
	case "warning":
		c, ic = p.Warning, IcWarning
	}
	return theme.Box(gtx, theme.Alpha(c, 0x1c), theme.Alpha(c, 0x55), 10, layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: 10, Top: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return th.Icon(gtx, rune(ic), 15, c) })
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return th.Paragraph(gtx, 14, p.Text, msg) }),
		)
	})
}

// ---- dialog --------------------------------------------------------------------------------------------------

// DAction is a dialog button; the last one is the primary action.
type DAction struct {
	Label string
	Kind  int
	Do    func()
	btn   Btn
}

// Dialog is a modal card over a dimmed window.
type Dialog struct {
	Title   string
	Width   unit.Dp
	Body    func(gtx layout.Context, th *theme.Theme) layout.Dimensions
	Actions []*DAction
	Close   func() // called on Esc / scrim
}

// Layout draws the scrim and the card; callers lay the underlying UI out with gtx.Disabled() first.
func (d *Dialog) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	p := th.P
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press && d.Close != nil {
			d.Close()
		}
	}
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, color.NRGBA{A: 0x99}, clip.Rect{Max: size}.Op())
	w := d.Width
	if w == 0 {
		w = 480
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		cw := gtx.Dp(w)
		if cw > size.X-gtx.Dp(32) {
			cw = size.X - gtx.Dp(32)
		}
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = cw, cw
		return theme.Box(gtx, p.Surface, p.BorderStrong, 16, layout.UniformInset(22), func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return Vert(gtx, 16,
				func(gtx layout.Context) layout.Dimensions { return th.Text(gtx, 17, font.Medium, p.Text, d.Title) },
				func(gtx layout.Context) layout.Dimensions { return d.Body(gtx, th) },
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx, d.actionChildren(gtx, th)...)
				})
		})
	})
}

func (d *Dialog) actionChildren(gtx layout.Context, th *theme.Theme) []layout.FlexChild {
	var out []layout.FlexChild
	for i, a := range d.Actions {
		a := a
		if a.btn.Clicked(gtx) && a.Do != nil {
			a.Do()
		}
		if i > 0 {
			out = append(out, layout.Rigid(layout.Spacer{Width: 8}.Layout))
		}
		out = append(out, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.btn.Layout(gtx, th, a.Label, a.Kind) }))
	}
	return out
}

// Short aliases used across the GUI code.
type (
	C  = layout.Context
	D  = layout.Dimensions
	Th = theme.Theme
)

func withA(c color.NRGBA, a uint8) color.NRGBA { return theme.Alpha(c, a) }
func theme0(c color.NRGBA) color.NRGBA         { return theme.Alpha(c, 0) }

var boxIn = theme.Box

// monoNudge is the downward shift of monospace text as a fraction of the font size.
const monoNudge = 0.12
