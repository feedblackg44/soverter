// Package gui contains the Gio-based windows of Soverter.
package gui

import (
	"gioui.org/op"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"soverter/internal/gui/theme"
)

// Button kinds.
const (
	Primary = iota
	Secondary
	Ghost
)

// Btn is a clickable button with a text label and an optional leading icon.
type Btn struct {
	widget.Clickable
	Icon rune
}

// Layout draws the button; kind selects the visual weight (one Primary per view).
func (b *Btn) Layout(gtx layout.Context, th *theme.Theme, label string, kind int) layout.Dimensions {
	p := th.P
	return b.Clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg, fg, border := p.Surface, p.Text, p.Border
		hover := b.Hovered() && gtx.Enabled()
		switch kind {
		case Primary:
			bg, fg, border = p.Accent, p.OnAccent, theme.Alpha(p.Accent, 0)
			if hover {
				bg = p.AccentHover
			}
		case Secondary:
			if hover {
				bg, border = p.Raised, p.BorderStrong
			}
		case Ghost:
			bg, fg, border = theme.Alpha(p.Surface, 0), p.TextDim, theme.Alpha(p.Surface, 0)
			if hover {
				bg, fg = p.Raised, p.Text
			}
		}
		if b.Pressed() && gtx.Enabled() {
			bg = p.Raised
		}
		if !gtx.Enabled() {
			bg, fg = theme.Alpha(p.Raised, 0xA0), p.TextMuted
			border = p.Border
			if kind == Ghost {
				bg = theme.Alpha(p.Surface, 0)
			}
		}
		return theme.Box(gtx, bg, border, 8, layout.Inset{Top: 10, Bottom: 10, Left: 16, Right: 16}, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if b.Icon == 0 {
						return layout.Dimensions{}
					}
					return layout.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return th.Icon(gtx, b.Icon, 14, fg)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Text(gtx, 14, font.Medium, fg, label) }),
			)
		})
	})
}

// Chip draws a small neutral tag.
func Chip(gtx layout.Context, th *theme.Theme, s string) layout.Dimensions {
	return theme.Box(gtx, th.P.Raised, th.P.Border, 6, layout.Inset{Top: 2, Bottom: 2, Left: 7, Right: 7}, func(gtx layout.Context) layout.Dimensions {
		return th.Text(gtx, 11, font.Medium, th.P.TextDim, s)
	})
}

// Progress draws a thin rounded progress bar; frac in 0..1.
func Progress(gtx layout.Context, th *theme.Theme, frac float64) layout.Dimensions {
	h := gtx.Dp(6)
	w := gtx.Constraints.Max.X
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	theme.Fill(gtx, th.P.Raised, image.Pt(w, h), 3)
	if fw := int(float64(w) * frac); fw > 0 {
		paint.FillShape(gtx.Ops, th.P.Accent, clip.UniformRRect(image.Rect(0, 0, fw, h), h/2).Op(gtx.Ops))
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// Spacer returns a fixed-height gap.
func Spacer(h unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(0, gtx.Dp(h))}
	}
}

func colorCall(gtx layout.Context, c color.NRGBA) op.CallOp {
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	return m.Stop()
}

func material_list(gtx layout.Context, l *widget.List, n int, w func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	return l.List.Layout(gtx, n, w)
}
