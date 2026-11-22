package gui

import (
	"gioui.org/font"
	"gioui.org/layout"

	"vidpreset/internal/gui/theme"
)

// QueueRow describes one queue item for display.
type QueueRow struct {
	File, Preset, Info string
	Frac               float64 // <0 = no progress shown
	State              string  // "running" | "pending" | "done" | "failed" | "canceled"
	Action             *Btn    // optional action button under the info line
	ActionLabel        string
}

// Layout draws a queue card.
func (r QueueRow) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	p := th.P
	stateCol := p.TextMuted
	switch r.State {
	case "running":
		stateCol = p.Accent
	case "done":
		stateCol = p.Success
	case "failed":
		stateCol = p.Danger
	}
	return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return theme.Box(gtx, p.Surface, p.Border, 12, layout.UniformInset(14), func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return Vert(gtx, 8,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return th.Text(gtx, 14, font.Medium, p.Text, r.File)
						}),
						layout.Rigid(layout.Spacer{Width: 8}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return Chip(gtx, th, r.Preset) }),
					)
				},
				func(gtx layout.Context) layout.Dimensions {
					if r.Frac < 0 {
						return layout.Dimensions{}
					}
					return Progress(gtx, th, r.Frac)
				},
				func(gtx layout.Context) layout.Dimensions {
					if r.State == "failed" {
						return th.Paragraph(gtx, 12, stateCol, r.Info)
					}
					return th.Text(gtx, 12, font.Normal, stateCol, r.Info)
				},
				func(gtx layout.Context) layout.Dimensions {
					if r.Action == nil {
						return layout.Dimensions{}
					}
					return r.Action.Layout(gtx, th, r.ActionLabel, Secondary)
				},
			)
		})
	})
}
