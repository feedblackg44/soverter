package gui

import (
	"fmt"
	"image"
	"io"
	"soverter/internal/i18n"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"soverter/internal/cmdbuild"
	"soverter/internal/preset"
)

func (e *Editor) setToast(s string) {
	e.toast = s
	e.toastUntil = time.Now().Add(4 * time.Second)
}

// Layout draws the whole editor window.
func (e *Editor) Layout(gtx C, th *Th) D {
	e.listInput(gtx)
	if e.loaded != e.sel || e.loaded == -2 {
		if e.sel >= len(e.cfg.Presets) {
			e.sel = len(e.cfg.Presets) - 1
		}
		e.loadFields()
		e.loaded = e.sel
	}
	if e.toast != "" {
		if time.Now().After(e.toastUntil) {
			e.toast = ""
		} else {
			gtx.Execute(op.InvalidateCmd{At: e.toastUntil})
		}
	}
	base := gtx
	if e.dlg != nil {
		base = gtx.Disabled()
	}
	dims := layout.Flex{}.Layout(base,
		layout.Rigid(func(gtx C) D { return e.sidebar(gtx, th) }),
		layout.Flexed(1, func(gtx C) D { return e.main(gtx, th) }),
	)
	if e.dlg != nil {
		e.dlg.Layout(gtx, th)
	}
	if e.pollFields() {
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

// ---- sidebar -------------------------------------------------------------------------------------------------

func (e *Editor) sidebar(gtx C, th *Th) D {
	p := th.P
	w := gtx.Dp(300)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
	paint.FillShape(gtx.Ops, p.Sidebar, clip.Rect{Max: image.Pt(w, gtx.Constraints.Max.Y)}.Op())
	paint.FillShape(gtx.Ops, p.Border, clip.Rect{Min: image.Pt(w-1, 0), Max: image.Pt(w, gtx.Constraints.Max.Y)}.Op())
	return layout.UniformInset(16).Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return Vert(gtx, 2,
					func(gtx C) D { return th.Text(gtx, 20, font.Medium, p.Text, "Soverter") },
					func(gtx C) D {
						return th.Text(gtx, 12, font.Normal, p.TextMuted, i18n.T("ПКМ по видео → готовый файл"))
					},
				)
			}),
			layout.Rigid(Spacer(20)),
			layout.Rigid(func(gtx C) D { return e.listToolbar(gtx, th) }),
			layout.Rigid(Spacer(8)),
			layout.Flexed(1, func(gtx C) D { return e.presetList(gtx, th) }),
			layout.Rigid(Spacer(8)),
			layout.Rigid(func(gtx C) D {
				e.bMenu.Icon, e.bSet.Icon = IcMenu, IcSettings
				if e.bMenu.Clicked(gtx) {
					e.menuDialog()
				}
				if e.bSet.Clicked(gtx) {
					e.settingsDialog()
				}
				return Vert(gtx, 4,
					func(gtx C) D { return e.bMenu.Layout(gtx, th, i18n.T("Меню Windows"), Ghost) },
					func(gtx C) D { return e.bSet.Layout(gtx, th, i18n.T("Параметры"), Ghost) })
			}),
		)
	})
}

func (e *Editor) listToolbar(gtx C, th *Th) D {
	if e.bAdd.Clicked(gtx) {
		e.addPreset()
	}
	if e.bDup.Clicked(gtx) {
		e.dupPreset()
	}
	if e.bDel.Clicked(gtx) {
		e.deletePreset()
	}
	if e.bUp.Clicked(gtx) {
		e.move(-1)
	}
	if e.bDown.Clicked(gtx) {
		e.move(1)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx C) D { return th.Text(gtx, 12, font.Medium, th.P.TextMuted, i18n.T("ПРЕСЕТЫ")) }),
		layout.Rigid(func(gtx C) D { return e.bAdd.Layout(gtx, th, IcAdd, false) }),
		layout.Rigid(func(gtx C) D { return e.bDup.Layout(gtx, th, IcCopy, false) }),
		layout.Rigid(func(gtx C) D { return e.bUp.Layout(gtx, th, IcUp, false) }),
		layout.Rigid(func(gtx C) D { return e.bDown.Layout(gtx, th, IcDown, false) }),
		layout.Rigid(func(gtx C) D { return e.bDel.Layout(gtx, th, IcDelete, true) }),
	)
}

func summary(p preset.Preset) string {
	if !p.Structured() {
		return i18n.T("сырые аргументы · .") + p.Suffix + "." + p.Ext
	}
	v, _ := p.Normalized()
	s := labelOf(codecOpts(), v.Codec)
	if v.Codec != preset.CodecCopy {
		s = strings.Fields(s)[0] + " · " + labelOf(familyNames(), v.Encoder)
	}
	return s + " · ." + p.Suffix + "." + p.Ext
}

// ---- main area -----------------------------------------------------------------------------------------------

func (e *Editor) main(gtx C, th *Th) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return e.topBar(gtx, th) }),
		layout.Flexed(1, func(gtx C) D { return e.content(gtx, th) }),
		layout.Rigid(func(gtx C) D { return e.statusBar(gtx, th) }),
	)
}

func (e *Editor) topBar(gtx C, th *Th) D {
	p := th.P
	e.bSave.Icon, e.bTest.Icon, e.bRevert.Icon, e.bImport.Icon = IcSave, IcPlay, IcUndo, IcImport
	if e.bSave.Clicked(gtx) {
		e.save()
	}
	if e.bRevert.Clicked(gtx) {
		e.revert()
	}
	if e.bTest.Clicked(gtx) {
		e.test()
	}
	if e.bImport.Clicked(gtx) {
		e.importDialog("")
	}
	hasPreset := e.cur() != nil
	return layout.Inset{Top: 18, Bottom: 14, Left: 24, Right: 24}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				title := i18n.T("Нет пресета")
				if c := e.cur(); c != nil {
					title = c.Name
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return th.Text(gtx, 20, font.Medium, p.Text, title) }),
					layout.Rigid(func(gtx C) D {
						if !e.dirty {
							return D{}
						}
						return layout.Inset{Left: 12}.Layout(gtx, func(gtx C) D {
							return boxIn(gtx, withA(p.Warning, 0x22), withA(p.Warning, 0x55), 6, layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}, func(gtx C) D {
								return th.Text(gtx, 11, font.Medium, p.Warning, i18n.T("не сохранено"))
							})
						})
					}),
				)
			}),
			layout.Rigid(func(gtx C) D {
				g := gtx
				if !hasPreset {
					g = gtx.Disabled()
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(g,
					layout.Rigid(func(gtx C) D { return e.bImport.Layout(gtx, th, i18n.T("Импорт"), Ghost) }),
					layout.Rigid(func(gtx C) D { return e.bTest.Layout(gtx, th, i18n.T("Тест"), Ghost) }),
				)
			}),
			layout.Rigid(func(gtx C) D {
				g := gtx
				if !e.dirty {
					g = gtx.Disabled()
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(g,
					layout.Rigid(func(gtx C) D { return e.bRevert.Layout(gtx, th, i18n.T("Сбросить"), Ghost) }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx C) D { return e.bSave.Layout(gtx, th, i18n.T("Сохранить"), Primary) }),
				)
			}),
		)
	})
}

func (e *Editor) statusBar(gtx C, th *Th) D {
	p := th.P
	w := gtx.Constraints.Max.X
	paint.FillShape(gtx.Ops, p.Border, clip.Rect{Max: image.Pt(w, 1)}.Op())
	return layout.Inset{Top: 8, Bottom: 8, Left: 24, Right: 24}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D { return th.Text(gtx, 12, font.Normal, p.TextMuted, e.statusText()) }),
			layout.Rigid(func(gtx C) D {
				if e.toast == "" {
					return D{}
				}
				return th.Text(gtx, 12, font.Medium, p.Success, e.toast)
			}),
		)
	})
}

func (e *Editor) content(gtx C, th *Th) D {
	c := e.cur()
	if c == nil {
		return layout.Center.Layout(gtx, func(gtx C) D {
			return th.Text(gtx, 14, font.Normal, th.P.TextMuted, i18n.T("Создайте пресет кнопкой «+» слева"))
		})
	}
	var items []layout.Widget
	items = append(items, func(gtx C) D { return e.general(gtx, th, c) })
	if c.Structured() {
		items = append(items,
			func(gtx C) D { return e.videoCard(gtx, th, c) },
			func(gtx C) D { return e.sizeCard(gtx, th, c) },
			func(gtx C) D { return e.audioCard(gtx, th, c) },
			func(gtx C) D { return e.extraCard(gtx, th) },
		)
	} else {
		items = append(items, func(gtx C) D { return e.rawCard(gtx, th) })
	}
	items = append(items, func(gtx C) D { return e.issuesBox(gtx, th) }, func(gtx C) D { return e.previewCard(gtx, th) })
	e.scroll.Axis = layout.Vertical
	return e.scroll.List.Layout(gtx, len(items), func(gtx C, i int) D {
		maxW := gtx.Dp(860)
		in := layout.Inset{Left: 24, Right: 24, Bottom: 14}
		if i == len(items)-1 {
			in.Bottom = 28
		}
		return in.Layout(gtx, func(gtx C) D {
			if gtx.Constraints.Max.X > maxW {
				gtx.Constraints.Max.X = maxW
			}
			return items[i](gtx)
		})
	})
}

// ---- cards ---------------------------------------------------------------------------------------------------

func (e *Editor) general(gtx C, th *Th, c *preset.Preset) D {
	return Section(gtx, th, i18n.T("Основное"), "", func(gtx C) D {
		return Vert(gtx, 14,
			Labeled(th, i18n.T("Название в меню"), func(gtx C) D {
				return e.fName.Layout(gtx, th, i18n.T("Как пункт называется в меню"))
			}),
			func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, Labeled(th, "id", func(gtx C) D { return e.fID.Layout(gtx, th, i18n.T("a-z, 0-9, дефис")) })),
					layout.Rigid(layout.Spacer{Width: 12}.Layout),
					layout.Flexed(1, Labeled(th, i18n.T("Суффикс (имя.суффикс.ext)"), func(gtx C) D { return e.fSuffix.Layout(gtx, th, "fhd") })),
				)
			},
			Labeled(th, i18n.T("Контейнер"), func(gtx C) D {
				opts := extOpts()
				if labelOf(opts, c.Ext) == "" && c.Ext != "" {
					opts = append(append([]Opt{}, opts...), Opt{Val: c.Ext, Label: c.Ext})
				}
				v, d := e.sExt.Layout(gtx, th, opts, c.Ext)
				if v != "" {
					e.setExt(v)
				}
				return d
			}),
		)
	})
}

func (e *Editor) rawCard(gtx C, th *Th) D {
	if e.bConvert.Clicked(gtx) {
		e.convertLegacy()
	}
	e.bConvert.Icon = IcImport
	return Section(gtx, th, i18n.T("Сырые аргументы"), "", func(gtx C) D {
		return Vert(gtx, 14,
			func(gtx C) D {
				return Banner(gtx, th, "info", i18n.T("Это пресет из сырых аргументов ffmpeg (старый формат). Он работает как раньше. Чтобы редактировать его в конструкторе, импортируйте его команду."))
			},
			Labeled(th, i18n.T("Видеофильтр (-vf)"), func(gtx C) D { return e.fRawVF.Layout(gtx, th, i18n.T("например scale=1920:1080,setsar=1")) }),
			Labeled(th, i18n.T("Аудиофильтр (-af)"), func(gtx C) D { return e.fRawAF.Layout(gtx, th, i18n.T("например volume=-6dB")) }),
			Labeled(th, i18n.T("Аргументы"), func(gtx C) D { return e.fRawArgs.Layout(gtx, th, "-c:v libx264 -crf 18 -c:a copy") }),
			func(gtx C) D {
				return e.bConvert.Layout(gtx, th, i18n.T("Открыть в конструкторе (через импорт)"), Secondary)
			},
		)
	})
}

func (e *Editor) videoCard(gtx C, th *Th, c *preset.Preset) D {
	v, _ := c.Normalized()
	copyV := v.Codec == preset.CodecCopy
	return Section(gtx, th, i18n.T("Видео"), "", func(gtx C) D {
		items := []layout.Widget{
			Labeled(th, i18n.T("Кодек"), func(gtx C) D {
				s, d := e.sCodec.Layout(gtx, th, codecOpts(), v.Codec)
				if s != "" {
					e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.Codec = s })
				}
				return d
			}),
		}
		if copyV {
			items = append(items, Hint(th, i18n.T("Видео копируется без перекодирования: быстро и без потерь, но размер и частоту кадров менять нельзя.")))
			return Vert(gtx, 14, items...)
		}
		items = append(items,
			func(gtx C) D { return e.encoderRow(gtx, th, v) },
			Labeled(th, i18n.T("Режим кодирования"), func(gtx C) D {
				s, d := e.sRate.Layout(gtx, th, rateOpts(), v.RateMode)
				if s != "" {
					e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.RateMode = s })
				}
				return d
			}),
		)
		switch v.RateMode {
		case preset.RateBitrate:
			items = append(items, Labeled(th, i18n.T("Битрейт, Мбит/с"), func(gtx C) D {
				return e.numField(gtx, th, e.fBitrate, i18n.T("например 5"), i18n.T("Мбит/с"))
			}))
		case preset.RateSize:
			items = append(items,
				Labeled(th, i18n.T("Целевой размер, МБ"), func(gtx C) D { return e.numField(gtx, th, e.fSizeMB, i18n.T("например 50"), i18n.T("МБ")) }),
				Hint(th, i18n.T("Битрейт считается при запуске по длительности файла и звуку (с запасом 5%). CPU H.264/H.265 кодируют в два прохода — точнее; аппаратные энкодеры и AV1 — в один, попадание в размер не гарантируется.")))
		default:
			items = append(items, Labeled(th, i18n.T("Качество"), func(gtx C) D { return e.qualityRow(gtx, th, v) }))
		}
		items = append(items, Labeled(th, i18n.T("Скорость кодирования"), func(gtx C) D { return e.speedRow(gtx, th, v) }))
		return Vert(gtx, 14, items...)
	})
}

func (e *Editor) numField(gtx C, th *Th, f *Field, hint, unit string) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			gtx.Constraints.Max.X = gtx.Dp(180)
			gtx.Constraints.Min.X = gtx.Dp(180)
			return f.Layout(gtx, th, hint)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 10}.Layout(gtx, func(gtx C) D { return th.Text(gtx, 13, font.Normal, th.P.TextDim, unit) })
		}),
	)
}

func (e *Editor) encoderRow(gtx C, th *Th, v preset.Video) D {
	rep, ready := e.encReport()
	var opts []Opt
	for _, f := range familyNames() {
		o := Opt{Val: f.Val, Label: f.Label}
		if ready && !rep.OK(cmdbuild.EncoderName(v.Codec, f.Val)) {
			o.Label += i18n.T(" (недоступно)")
			o.Off = f.Val != v.Encoder
		}
		opts = append(opts, o)
	}
	return Labeled(th, i18n.T("Энкодер"), func(gtx C) D {
		return Vert(gtx, 8,
			func(gtx C) D {
				s, d := e.sEnc.Layout(gtx, th, opts, v.Encoder)
				if s != "" {
					e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.Encoder = s })
				}
				return d
			},
			Hint(th, e.encoderHint(v.Codec, v.Encoder)))
	})(gtx)
}

func (e *Editor) qualityRow(gtx C, th *Th, v preset.Video) D {
	p := th.P
	return Vert(gtx, 6,
		func(gtx C) D {
			nv, ch, d := e.slider.Layout(gtx, th, v.Quality, 51)
			if ch && nv != v.Quality {
				e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.Quality = nv })
			}
			return d
		},
		func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return th.Text(gtx, 12, font.Normal, p.TextMuted, i18n.T("← лучше качество"))
				}),
				layout.Flexed(1, func(gtx C) D {
					return layout.Center.Layout(gtx, func(gtx C) D {
						return th.Text(gtx, 14, font.Medium, p.Accent, fmt.Sprintf("%d", v.Quality))
					})
				}),
				layout.Rigid(func(gtx C) D {
					return th.Text(gtx, 12, font.Normal, p.TextMuted, i18n.T("меньше размер →"))
				}),
			)
		},
	)
}

func (e *Editor) speedRow(gtx C, th *Th, v preset.Video) D {
	opts := speedOpts()
	if labelOf(opts, v.Speed) == "" && v.Speed != "" {
		opts = append(append([]Opt{}, opts...), Opt{Val: v.Speed, Label: v.Speed + i18n.T(" (из импорта)")})
	}
	return Vert(gtx, 6,
		func(gtx C) D {
			s, d := e.sSpeed.Layout(gtx, th, opts, v.Speed)
			if e.sSpeed.Hit {
				e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.Speed = s })
			}
			return d
		},
		func(gtx C) D {
			if v.Speed == "" {
				return D{}
			}
			flag := "-preset"
			if v.Encoder == preset.EncAMF {
				flag = "-quality"
			}
			return th.Text(gtx, 12, font.Normal, th.P.TextMuted, "→ "+flag+" "+cmdbuild.SpeedValue(v.Codec, v.Encoder, v.Speed))
		},
	)
}

func (e *Editor) sizeCard(gtx C, th *Th, c *preset.Preset) D {
	v, _ := c.Normalized()
	return Section(gtx, th, i18n.T("Размер кадра и fps"), i18n.T("Масштабирование выполняется на CPU"), func(gtx C) D {
		if v.Codec == preset.CodecCopy {
			return Hint(th, i18n.T("Без перекодирования размер и частоту кадров менять нельзя."))(gtx)
		}
		key := sizeKeyFor(v)
		if e.sizeCustom && key != sizeNone {
			key = sizeCustom
		}
		items := []layout.Widget{
			Labeled(th, i18n.T("Размер"), func(gtx C) D {
				s, d := e.sSize.Layout(gtx, th, sizeOpts(), key)
				if s != "" {
					e.pickSize(s)
				}
				return d
			}),
		}
		if key != sizeNone {
			items = append(items, Labeled(th, i18n.T("Как масштабировать"), func(gtx C) D {
				s, d := e.sScale.Layout(gtx, th, scaleModeOpts(), v.ScaleMode)
				if s != "" {
					e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.ScaleMode = s })
				}
				return d
			}))
		}
		if key == sizeCustom {
			items = append(items, func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, Labeled(th, i18n.T("Ширина"), func(gtx C) D { return e.fW.Layout(gtx, th, "px") })),
					layout.Rigid(layout.Spacer{Width: 12}.Layout),
					layout.Flexed(1, Labeled(th, i18n.T("Высота"), func(gtx C) D { return e.fH.Layout(gtx, th, "px") })),
				)
			})
			if v.ScaleMode == preset.ScaleKeep {
				items = append(items, Hint(th, i18n.T("При сохранении пропорций важна только высота: ширина считается сама.")))
			}
		}
		items = append(items, Labeled(th, i18n.T("Частота кадров, fps"), func(gtx C) D {
			opts := fpsOpts()
			sel := numOrEmpty(v.FPS)
			if labelOf(opts, sel) == "" {
				opts = append(append([]Opt{}, opts...), Opt{Val: sel, Label: sel})
			}
			return Vert(gtx, 8,
				func(gtx C) D {
					s, d := e.sFPS.Layout(gtx, th, opts, sel)
					if e.sFPS.Hit {
						n := parseNum(s)
						e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.FPS = n })
						e.fFPS.Set(s)
					}
					return d
				},
				func(gtx C) D {
					return e.numField(gtx, th, e.fFPS, i18n.T("другое значение"), "fps")
				})
		}))
		return Vert(gtx, 14, items...)
	})
}

// anyClicked is a placeholder-free check that the segmented control produced a click this frame.
func (e *Editor) anyClicked(_ *Seg, _ int) bool { return true }

func (e *Editor) pickSize(key string) {
	e.sizeCustom = key == sizeCustom
	e.editVideo(func(v *preset.Video, _ *preset.Audio) {
		if key == sizeNone {
			v.ScaleMode, v.Width, v.Height = preset.ScaleNone, 0, 0
			return
		}
		if v.ScaleMode == preset.ScaleNone || v.ScaleMode == "" {
			v.ScaleMode = preset.ScaleKeep
		}
		if key == sizeCustom {
			return
		}
		for _, s := range sizePresets() {
			if s.val == key {
				v.Width, v.Height = s.w, s.h
			}
		}
	})
	v, _ := e.cur().Normalized()
	e.fW.Set(intOrEmpty(v.Width))
	e.fH.Set(intOrEmpty(v.Height))
}

func (e *Editor) audioCard(gtx C, th *Th, c *preset.Preset) D {
	_, a := c.Normalized()
	return Section(gtx, th, i18n.T("Звук"), "", func(gtx C) D {
		items := []layout.Widget{
			Labeled(th, i18n.T("Обработка звука"), func(gtx C) D {
				s, d := e.sAMode.Layout(gtx, th, audioOpts(), a.Mode)
				if s != "" {
					e.editVideo(func(_ *preset.Video, a *preset.Audio) { a.Mode = s })
				}
				return d
			}),
		}
		if a.Mode == preset.AudioAAC || a.Mode == preset.AudioOpus {
			items = append(items, Labeled(th, i18n.T("Битрейт звука"), func(gtx C) D {
				opts := audioBitrates()
				if labelOf(opts, a.Bitrate) == "" && a.Bitrate != "" {
					opts = append(append([]Opt{}, opts...), Opt{Val: a.Bitrate, Label: a.Bitrate})
				}
				s, d := e.sABit.Layout(gtx, th, opts, a.Bitrate)
				if s != "" {
					e.editVideo(func(_ *preset.Video, a *preset.Audio) { a.Bitrate = s })
				}
				return d
			}))
		}
		if a.Mode != preset.AudioRemove {
			items = append(items, Labeled(th, i18n.T("Громкость, дБ (0 = не менять)"), func(gtx C) D { return e.numField(gtx, th, e.fVol, i18n.T("например -6"), i18n.T("дБ")) }))
		}
		switch {
		case a.VolumeDB != 0 && a.Mode != preset.AudioRemove:
			items = append(items, Hint(th, i18n.T("Изменение громкости требует перекодирования звука, поэтому звук не копируется как есть.")))
		}
		return Vert(gtx, 14, items...)
	})
}

func (e *Editor) extraCard(gtx C, th *Th) D {
	return Section(gtx, th, i18n.T("Дополнительно"), i18n.T("Всё, чего нет в форме. Идёт в конец команды, перед выходным файлом."), func(gtx C) D {
		return Vert(gtx, 14,
			Labeled(th, i18n.T("Видеофильтры (-vf)"), func(gtx C) D { return e.fVF.Layout(gtx, th, i18n.T("например hqdn3d")) }),
			Labeled(th, i18n.T("Аудиофильтры (-af)"), func(gtx C) D { return e.fAF.Layout(gtx, th, "") }),
			Labeled(th, i18n.T("Дополнительные аргументы"), func(gtx C) D { return e.fArgs.Layout(gtx, th, i18n.T("например -movflags +faststart")) }),
		)
	})
}

func (e *Editor) issuesBox(gtx C, th *Th) D {
	if len(e.issues) == 0 {
		return D{}
	}
	var items []layout.Widget
	for _, is := range e.issues {
		is := is
		kind := "warning"
		if is.Severity == cmdbuild.Error {
			kind = "error"
		}
		items = append(items, func(gtx C) D { return Banner(gtx, th, kind, is.Msg) })
	}
	return Vert(gtx, 8, items...)
}

func (e *Editor) previewCard(gtx C, th *Th) D {
	e.bCopy.Icon = IcCopy
	if e.bCopy.Clicked(gtx) && e.previewText != "" {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(e.previewText))})
		e.setToast(i18n.T("Команда скопирована"))
	}
	p := th.P
	return Section(gtx, th, i18n.T("Итоговая команда"), i18n.T("Обновляется по мере правок"), func(gtx C) D {
		return Vert(gtx, 12,
			func(gtx C) D {
				return boxIn(gtx, p.Bg, p.Border, 10, layout.UniformInset(12), func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					if e.previewText == "" {
						return th.Paragraph(gtx, 13, p.TextMuted, i18n.T("Команду собрать нельзя: исправьте ошибки выше."))
					}
					return th.Code(gtx, 13, p.Text, e.previewText)
				})
			},
			func(gtx C) D {
				return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return e.bCopy.Layout(gtx, th, i18n.T("Копировать"), Secondary) }))
			},
		)
	})
}

var _ = unit.Dp(0)
