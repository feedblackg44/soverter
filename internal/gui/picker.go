package gui

import (
	"fmt"
	"image"
	"soverter/internal/i18n"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"soverter/internal/gui/theme"
	"soverter/internal/preset"
)

// PickerView is a command-palette style preset chooser: type to filter, arrows to move, Enter to run.
type PickerView struct {
	Presets []preset.Preset
	OnPick  func(preset.Preset)
	OnClose func()
	OnInfo  func() // if set, a "video properties" button is shown

	info   Btn
	search widget.Editor
	list   widget.List
	rows   []widget.Clickable
	sel    int
	inited bool
}

func (v *PickerView) filtered() []preset.Preset {
	q := strings.ToLower(strings.TrimSpace(v.search.Text()))
	if q == "" {
		return v.Presets
	}
	var out []preset.Preset
	for _, p := range v.Presets {
		if strings.Contains(strings.ToLower(p.Name+" "+p.Suffix+" "+p.Ext), q) {
			out = append(out, p)
		}
	}
	return out
}

// Layout draws the picker and handles input.
func (v *PickerView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !v.inited {
		v.inited = true
		v.search.SingleLine = true
		v.search.Submit = true
		v.list.Axis = layout.Vertical
		gtx.Execute(key.FocusCmd{Tag: &v.search})
	}
	items := v.filtered()
	if len(v.rows) < len(v.Presets) {
		v.rows = make([]widget.Clickable, len(v.Presets))
	}
	if v.sel >= len(items) {
		v.sel = len(items) - 1
	}
	if v.sel < 0 {
		v.sel = 0
	}
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &v.search, Name: key.NameUpArrow},
			key.Filter{Focus: &v.search, Name: key.NameDownArrow},
			key.Filter{Focus: &v.search, Name: key.NameEscape},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			switch e.Name {
			case key.NameUpArrow:
				if v.sel > 0 {
					v.sel--
				}
			case key.NameDownArrow:
				if v.sel < len(items)-1 {
					v.sel++
				}
			case key.NameEscape:
				if v.OnClose != nil {
					v.OnClose()
				}
			}
		}
	}
	for {
		ev, ok := v.search.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok && len(items) > 0 && v.OnPick != nil {
			v.OnPick(items[v.sel])
		}
	}

	p := th.P
	paint.FillShape(gtx.Ops, p.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return theme.Box(gtx, p.Surface, p.BorderStrong, 10, layout.Inset{Top: 12, Bottom: 12, Left: 14, Right: 14}, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return th.Text(gtx, 15, font.Normal, p.Accent, "⌕")
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							dims := v.search.Layout(gtx, th.Shaper, th.Font(font.Normal), 15, colorCall(gtx, p.Text), colorCall(gtx, p.AccentWash))
							if v.search.Len() == 0 {
								th.Text(gtx, 15, font.Normal, p.TextMuted, i18n.T("Выберите пресет…"))
							}
							return dims
						}),
					)
				})
			}),
			layout.Rigid(Spacer(10)),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(items) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return th.Text(gtx, 13, font.Normal, p.TextMuted, i18n.T("Ничего не найдено"))
					})
				}
				return material_list(gtx, &v.list, len(items), func(gtx layout.Context, i int) layout.Dimensions {
					return v.row(gtx, th, items, i)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if v.OnInfo == nil {
					return layout.Dimensions{}
				}
				if v.info.Clicked(gtx) {
					v.OnInfo()
				}
				v.info.Icon = IcInfo
				return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return v.info.Layout(gtx, th, i18n.T("Свойства видео"), Ghost)
				})
			}),
		)
	})
}

func (v *PickerView) row(gtx layout.Context, th *theme.Theme, items []preset.Preset, i int) layout.Dimensions {
	p := th.P
	pr := items[i]
	idx := 0
	for j := range v.Presets {
		if v.Presets[j].ID == pr.ID {
			idx = j
		}
	}
	btn := &v.rows[idx]
	if btn.Clicked(gtx) && v.OnPick != nil {
		v.OnPick(pr)
	}
	return layout.Inset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bg, border := theme.Alpha(p.Surface, 0), theme.Alpha(p.Surface, 0)
			if i == v.sel {
				bg, border = p.AccentWash, theme.Alpha(p.Accent, 0x80)
			} else if btn.Hovered() {
				bg = p.Surface
			}
			ext := pr.Ext
			return theme.Box(gtx, bg, border, 10, layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return th.Text(gtx, 14, font.Medium, p.Text, pr.Name)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return th.Text(gtx, 12, font.Normal, p.TextMuted, fmt.Sprintf("…%s.%s", pr.Suffix, ext))
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if pr.Video != nil {
							return Chip(gtx, th, strings.ToUpper(pr.Video.Codec))
						}
						return layout.Dimensions{Size: image.Pt(0, 0)}
					}),
				)
			})
		})
	})
}

// OpenPicker opens the preset chooser; onPick runs (in the UI goroutine) before the window closes.
func OpenPicker(presets []preset.Preset, onPick func(preset.Preset), onInfo func()) *Win {
	v := &PickerView{Presets: presets}
	var w *Win
	v.OnPick = func(p preset.Preset) {
		onPick(p)
		w.Close()
	}
	v.OnClose = func() { w.Close() }
	if onInfo != nil {
		v.OnInfo = onInfo // the properties open in their own window; the picker stays
	}
	w = Open(i18n.T("Выберите пресет"), 460, 480, v)
	return w
}
