// Package theme is the vidpreset design system on top of Gio: color tokens, fonts and small drawing helpers.
package theme

import (
	"image"
	"image/color"
	"os"
	"path/filepath"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Palette holds every color the UI uses. Nothing outside this struct hardcodes a color.
type Palette struct {
	Bg, Surface, Raised, Sidebar  color.NRGBA // canvas, cards, hovered/raised cards, side panel
	Border, BorderStrong          color.NRGBA
	Text, TextDim, TextMuted      color.NRGBA
	Accent, AccentHover, OnAccent color.NRGBA
	AccentWash                    color.NRGBA // translucent accent for selection
	Danger, Success, Warning      color.NRGBA
}

func rgb(h uint32) color.NRGBA {
	return color.NRGBA{R: uint8(h >> 16), G: uint8(h >> 8), B: uint8(h), A: 0xff}
}
func rgba(h uint32, a uint8) color.NRGBA {
	c := rgb(h)
	c.A = a
	return c
}

// DarkPalette is "Graphite + indigo".
func DarkPalette() Palette {
	return Palette{
		Bg: rgb(0x14151A), Surface: rgb(0x1C1E26), Raised: rgb(0x242733), Sidebar: rgb(0x101116),
		Border: rgba(0xFFFFFF, 0x12), BorderStrong: rgba(0xFFFFFF, 0x26),
		Text: rgb(0xE6E8F0), TextDim: rgb(0xA0A5B8), TextMuted: rgb(0x6B7085),
		Accent: rgb(0x6E7BFF), AccentHover: rgb(0x8590FF), OnAccent: rgb(0x0B0D1F), AccentWash: rgba(0x6E7BFF, 0x26),
		Danger: rgb(0xFF6B6B), Success: rgb(0x4ADE9A), Warning: rgb(0xF5B84B),
	}
}

// LightPalette is the light twin of DarkPalette.
func LightPalette() Palette {
	return Palette{
		Bg: rgb(0xF4F5F8), Surface: rgb(0xFFFFFF), Raised: rgb(0xEEF0F6), Sidebar: rgb(0xEAECF2),
		Border: rgba(0x14151A, 0x14), BorderStrong: rgba(0x14151A, 0x2E),
		Text: rgb(0x16181F), TextDim: rgb(0x555A6E), TextMuted: rgb(0x8A8FA3),
		Accent: rgb(0x4F5DF0), AccentHover: rgb(0x3F4CD6), OnAccent: rgb(0xFFFFFF), AccentWash: rgba(0x4F5DF0, 0x1A),
		Danger: rgb(0xD64545), Success: rgb(0x178F5B), Warning: rgb(0xB7791F),
	}
}

// Theme bundles the palette with the text shaper.
type Theme struct {
	P      Palette
	Shaper *text.Shaper
	Face   font.Typeface
}

// New loads Segoe UI Variable/Segoe UI from the system (always present on Windows) and falls back to Go fonts.
func New(p Palette) *Theme {
	var coll []font.FontFace
	face := font.Typeface("Segoe UI")
	dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
	for _, name := range []string{"segoeui.ttf", "segoeuib.ttf", "seguisb.ttf", "segoeuil.ttf"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if f, err := opentype.ParseCollection(b); err == nil {
			coll = append(coll, f...)
		}
	}
	if len(coll) == 0 {
		coll = gofont.Collection()
		face = ""
	}
	return &Theme{P: p, Shaper: text.NewShaper(text.WithCollection(coll)), Face: face}
}

// Font returns a font of the theme typeface with the given weight.
func (t *Theme) Font(w font.Weight) font.Font { return font.Font{Typeface: t.Face, Weight: w} }

func colorOp(gtx layout.Context, c color.NRGBA) op.CallOp {
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	return m.Stop()
}

// Text draws a single-line label.
func (t *Theme) Text(gtx layout.Context, size float32, w font.Weight, c color.NRGBA, s string) layout.Dimensions {
	return widget.Label{MaxLines: 1, Truncator: "…"}.Layout(gtx, t.Shaper, t.Font(w), unit.Sp(size), s, colorOp(gtx, c))
}

// Paragraph draws wrapping text.
func (t *Theme) Paragraph(gtx layout.Context, size float32, c color.NRGBA, s string) layout.Dimensions {
	return widget.Label{}.Layout(gtx, t.Shaper, t.Font(font.Normal), unit.Sp(size), s, colorOp(gtx, c))
}

// Fill paints a rounded rectangle of the given size.
func Fill(gtx layout.Context, c color.NRGBA, size image.Point, radius unit.Dp) {
	r := gtx.Dp(radius)
	paint.FillShape(gtx.Ops, c, clip.UniformRRect(image.Rectangle{Max: size}, r).Op(gtx.Ops))
}

// Outline strokes a rounded rectangle border.
func Outline(gtx layout.Context, c color.NRGBA, size image.Point, radius unit.Dp, width unit.Dp) {
	r := gtx.Dp(radius)
	w := float32(gtx.Dp(width))
	if w < 1 {
		w = 1
	}
	rr := clip.UniformRRect(image.Rectangle{Max: size}, r)
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: rr.Path(gtx.Ops), Width: w}.Op())
}

// Box lays out w on a rounded background with optional border; padding is in dp.
func Box(gtx layout.Context, bg, border color.NRGBA, radius unit.Dp, pad layout.Inset, w layout.Widget) layout.Dimensions {
	rec := op.Record(gtx.Ops)
	dims := pad.Layout(gtx, w)
	call := rec.Stop()
	Fill(gtx, bg, dims.Size, radius)
	if border.A != 0 {
		Outline(gtx, border, dims.Size, radius, 1)
	}
	call.Add(gtx.Ops)
	return dims
}

// Alpha returns c with multiplied opacity.
func Alpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = uint8(uint16(c.A) * uint16(a) / 255)
	return c
}
