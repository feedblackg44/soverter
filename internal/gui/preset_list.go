package gui

import (
	"image"
	"math"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"vidpreset/internal/i18n"
)

type presetRow struct {
	click  widget.Clickable
	on     Switch
	drag   gesture.Drag
	pressY float32
}

// listInput handles row clicks and switches before anything is drawn, so every row of the frame sees the
// new selection (otherwise the previously selected row would stay highlighted until the next frame).
func (e *Editor) listInput(gtx C) {
	for len(e.rows) < len(e.cfg.Presets) {
		e.rows = append(e.rows, presetRow{})
	}
	for i := range e.cfg.Presets {
		row := &e.rows[i]
		if row.click.Clicked(gtx) && e.sel != i && e.dragIdx < 0 {
			e.selectPreset(i)
		}
		if row.on.Update(gtx) {
			e.cfg.Presets[i].Disabled = !row.on.Value
			e.recomputeDirty()
		}
	}
}

// dragTarget is the index the dragged row would be dropped at.
func (e *Editor) dragTarget() int {
	if e.dragIdx < 0 || e.rowPitch <= 0 {
		return -1
	}
	t := e.dragIdx + int(math.Round(float64(e.dragDY)/float64(e.rowPitch)))
	if t < 0 {
		t = 0
	}
	if t >= len(e.cfg.Presets) {
		t = len(e.cfg.Presets) - 1
	}
	return t
}

func (e *Editor) finishDrag() {
	i, t := e.dragIdx, e.dragTarget()
	e.dragIdx, e.dragDY = -1, 0
	if i < 0 || t < 0 || t == i {
		return
	}
	ps := e.cfg.Presets
	p := ps[i]
	if i < t {
		copy(ps[i:t], ps[i+1:t+1])
	} else {
		copy(ps[t+1:i+1], ps[t:i])
	}
	ps[t] = p
	e.sel = t
	e.recomputeDirty()
}

func (e *Editor) presetList(gtx C, th *Th) D {
	p := th.P
	if len(e.cfg.Presets) == 0 {
		return th.Paragraph(gtx, 13, p.TextMuted, i18n.T("Пока нет ни одного пресета. Нажмите «+», чтобы создать."))
	}
	return e.list.List.Layout(gtx, len(e.cfg.Presets), func(gtx C, i int) D {
		pr := &e.cfg.Presets[i]
		row := &e.rows[i]
		row.on.Value = !pr.Disabled

		dragging := e.dragIdx == i
		rec := op.Record(gtx.Ops)
		dims := layout.Inset{Bottom: 4}.Layout(gtx, func(gtx C) D {
			return row.click.Layout(gtx, func(gtx C) D {
				bg, border := theme0(p.Surface), theme0(p.Surface)
				switch {
				case dragging:
					bg, border = p.Raised, p.Accent
				case i == e.sel:
					bg, border = p.AccentWash, withA(p.Accent, 0x70)
				case row.click.Hovered() && e.dragIdx < 0:
					bg = p.Surface
				}
				nameC, subC := p.Text, p.TextMuted
				if pr.Disabled {
					nameC, subC = p.TextMuted, withA(p.TextMuted, 0xA0)
				}
				return boxIn(gtx, bg, border, 10, layout.Inset{Top: 9, Bottom: 9, Left: 12, Right: 10}, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D {
							return Vert(gtx, 2,
								func(gtx C) D { return th.Text(gtx, 14, font.Medium, nameC, pr.Name) },
								func(gtx C) D { return th.Text(gtx, 11, font.Normal, subC, summary(*pr)) })
						}),
						layout.Rigid(func(gtx C) D { return row.on.Layout(gtx, th) }),
					)
				})
			})
		})
		call := rec.Stop()
		e.rowPitch = dims.Size.Y

		if dragging {
			// the row follows the pointer and is painted above the others
			st := op.Offset(image.Pt(0, int(e.dragDY))).Push(gtx.Ops)
			op.Defer(gtx.Ops, call)
			st.Pop()
		} else {
			call.Add(gtx.Ops)
		}

		// drag gesture: registered at the row's resting place, passing events on so click and switch keep working
		area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
		pass := pointer.PassOp{}.Push(gtx.Ops)
		row.drag.Add(gtx.Ops)
		pass.Pop()
		area.Pop()
		for {
			ev, ok := row.drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
			if !ok {
				break
			}
			switch ev.Kind {
			case pointer.Press:
				row.pressY = ev.Position.Y
			case pointer.Drag:
				dy := ev.Position.Y - row.pressY
				if e.dragIdx != i && math.Abs(float64(dy)) > float64(gtx.Dp(unit.Dp(5))) {
					e.dragIdx = i
					if e.sel != i {
						e.selectPreset(i)
					}
				}
				if e.dragIdx == i {
					e.dragDY = dy
				}
			case pointer.Release, pointer.Cancel:
				if e.dragIdx == i {
					e.finishDrag()
				}
			}
		}
		if t := e.dragTarget(); t == i && e.dragIdx >= 0 && t != e.dragIdx {
			y := 0
			if t > e.dragIdx {
				y = dims.Size.Y - gtx.Dp(4)
			}
			paint.FillShape(gtx.Ops, p.Accent, clip.UniformRRect(image.Rect(gtx.Dp(6), y-1, dims.Size.X-gtx.Dp(6), y+gtx.Dp(2)), 2).Op(gtx.Ops))
		}
		return dims
	})
}
