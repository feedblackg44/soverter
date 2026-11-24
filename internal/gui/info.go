package gui

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"

	"soverter/internal/gui/theme"
	"soverter/internal/i18n"
	"soverter/internal/mediainfo"
)

// infoSection is a titled group of label/value rows ("General", "Video 1", ...).
type infoSection struct {
	Title string
	Rows  [][2]string
}

// infoSections turns the probe result into displayable rows. Pure, so it is covered by tests.
func infoSections(in mediainfo.Info) []infoSection {
	var out []infoSection
	gen := infoSection{Title: i18n.T("Общее")}
	add := func(s *infoSection, label, value string) {
		if value != "" {
			s.Rows = append(s.Rows, [2]string{label, value})
		}
	}
	add(&gen, i18n.T("Контейнер"), in.Container)
	add(&gen, i18n.T("Размер"), fmtSize(in.Size))
	add(&gen, i18n.T("Длительность"), fmtDuration(in.Duration))
	add(&gen, i18n.T("Общий битрейт"), fmtBitrate(in.BitRate))
	if in.Subtitles > 0 {
		add(&gen, i18n.T("Субтитры"), fmt.Sprint(in.Subtitles))
	}
	out = append(out, gen)

	var audioKnown float64
	for _, a := range in.Audio {
		audioKnown += a.BitRate
	}
	for i, v := range in.Video {
		title := i18n.T("Видео")
		if len(in.Video) > 1 {
			title += fmt.Sprintf(" %d", i+1)
		}
		s := infoSection{Title: title}
		codec := v.Codec
		if v.Profile != "" {
			codec += " (" + v.Profile + ")"
		}
		add(&s, i18n.T("Кодек"), codec)
		if v.Width > 0 && v.Height > 0 {
			res := fmt.Sprintf("%d×%d", v.Width, v.Height)
			if v.Aspect != "" {
				res += " · " + v.Aspect
			}
			add(&s, i18n.T("Разрешение"), res)
		}
		if v.FPS > 0 {
			add(&s, i18n.T("Частота кадров"), fmt.Sprintf("%s fps", trimFloat(v.FPS)))
		}
		switch {
		case v.BitRate > 0:
			add(&s, i18n.T("Битрейт"), fmtBitrate(v.BitRate))
		case len(in.Video) == 1 && in.BitRate > audioKnown && audioKnown >= 0:
			// containers like mkv/webm often carry no per-stream bitrate: overall minus audio is a fair estimate
			add(&s, i18n.T("Битрейт"), "≈ "+fmtBitrate(in.BitRate-audioKnown)+i18n.T(" (оценка)"))
		}
		pix := v.PixFmt
		if v.BitDepth > 0 {
			pix += fmt.Sprintf(i18n.T(", %d бит"), v.BitDepth)
		}
		add(&s, i18n.T("Формат пикселей"), pix)
		add(&s, "HDR", v.HDR)
		if v.Interlaced {
			add(&s, i18n.T("Развёртка"), i18n.T("чересстрочная"))
		}
		if v.Rotation != 0 {
			add(&s, i18n.T("Поворот"), fmt.Sprintf("%d°", v.Rotation))
		}
		out = append(out, s)
	}
	for i, a := range in.Audio {
		title := i18n.T("Аудио")
		if len(in.Audio) > 1 {
			title += fmt.Sprintf(" %d", i+1)
		}
		s := infoSection{Title: title}
		codec := a.Codec
		if a.Profile != "" {
			codec += " (" + a.Profile + ")"
		}
		add(&s, i18n.T("Кодек"), codec)
		ch := ""
		switch {
		case a.Layout != "":
			ch = a.Layout
		case a.Channels > 0:
			ch = fmt.Sprint(a.Channels)
		}
		add(&s, i18n.T("Каналы"), ch)
		if a.SampleRate > 0 {
			add(&s, i18n.T("Частота дискретизации"), fmt.Sprintf("%s kHz", trimFloat(float64(a.SampleRate)/1000)))
		}
		add(&s, i18n.T("Битрейт"), fmtBitrate(a.BitRate))
		add(&s, i18n.T("Язык"), a.Language)
		out = append(out, s)
	}
	return out
}

// infoText is the plain-text form of the sections, for the Copy button.
func infoText(in mediainfo.Info) string {
	var b strings.Builder
	b.WriteString(filepath.Base(in.Path) + "\n")
	for _, s := range infoSections(in) {
		b.WriteString("\n" + s.Title + "\n")
		for _, r := range s.Rows {
			b.WriteString("  " + r[0] + ": " + r[1] + "\n")
		}
	}
	return b.String()
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%.3f", f)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func fmtSize(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n >= 1<<30:
		return fmt.Sprintf("%.2f %s", float64(n)/(1<<30), i18n.T("ГБ"))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f %s", float64(n)/(1<<20), i18n.T("МБ"))
	}
	return fmt.Sprintf("%.0f %s", float64(n)/(1<<10), i18n.T("КБ"))
}

func fmtBitrate(bps float64) string {
	switch {
	case bps <= 0:
		return ""
	case bps >= 1e6:
		return fmt.Sprintf("%.2f %s", bps/1e6, i18n.T("Мбит/с"))
	}
	return fmt.Sprintf("%.0f %s", bps/1e3, i18n.T("кбит/с"))
}

func fmtDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	s := int(d.Round(time.Second).Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// ---- window --------------------------------------------------------------------------------------------------

type infoFile struct {
	path string
	info *mediainfo.Info
	err  string
	copy Btn
}

// InfoView lists the properties of one or more files, each read with ffprobe in the background.
type InfoView struct {
	mu    sync.Mutex
	files []*infoFile
	list  widget.List
	win   *Win
	toast string
	until time.Time
}

// OpenInfo opens the properties window and starts probing the files.
func OpenInfo(ffprobe string, files []string) *Win {
	v := &InfoView{}
	v.list.Axis = layout.Vertical
	for _, f := range files {
		v.files = append(v.files, &infoFile{path: f})
	}
	title := i18n.T("Свойства видео")
	if len(files) == 1 {
		title += " — " + filepath.Base(files[0])
	}
	v.win = Open(title, 640, 760, v)
	go func() {
		for _, f := range v.files {
			in, err := mediainfo.Probe(context.Background(), ffprobe, f.path)
			v.mu.Lock()
			if err != nil {
				f.err = err.Error()
			} else {
				f.info = &in
			}
			v.mu.Unlock()
			v.win.Invalidate()
		}
	}()
	return v.win
}

// Layout draws one card per file.
func (v *InfoView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	defer v.mu.Unlock()
	return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return v.list.List.Layout(gtx, len(v.files), func(gtx layout.Context, i int) layout.Dimensions {
			return layout.Inset{Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return v.card(gtx, th, v.files[i])
			})
		})
	})
}

func (v *InfoView) card(gtx layout.Context, th *theme.Theme, f *infoFile) layout.Dimensions {
	p := th.P
	f.copy.Icon = IcCopy
	if f.copy.Clicked(gtx) && f.info != nil {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(infoText(*f.info)))})
	}
	return theme.Box(gtx, p.Surface, p.Border, 14, layout.UniformInset(18), func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		head := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return Vert(gtx, 2,
						func(gtx layout.Context) layout.Dimensions {
							return th.Text(gtx, 16, font.Medium, p.Text, filepath.Base(f.path))
						},
						func(gtx layout.Context) layout.Dimensions {
							return th.Text(gtx, 11, font.Normal, p.TextMuted, filepath.Dir(f.path))
						})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if f.info == nil {
						return layout.Dimensions{}
					}
					return f.copy.Layout(gtx, th, i18n.T("Копировать"), Secondary)
				}),
			)
		}
		switch {
		case f.err != "":
			return Vert(gtx, 12, head, func(gtx layout.Context) layout.Dimensions {
				return Banner(gtx, th, "error", i18n.T("ffprobe не смог прочитать файл: ")+f.err)
			})
		case f.info == nil:
			return Vert(gtx, 12, head, func(gtx layout.Context) layout.Dimensions {
				return th.Text(gtx, 13, font.Normal, p.TextMuted, i18n.T("Читаю файл…"))
			})
		}
		rows := []layout.Widget{head}
		for _, s := range infoSections(*f.info) {
			s := s
			rows = append(rows, func(gtx layout.Context) layout.Dimensions { return infoGroup(gtx, th, s) })
		}
		return Vert(gtx, 16, rows...)
	})
}

// infoGroup draws a small heading and its label/value rows.
func infoGroup(gtx layout.Context, th *theme.Theme, s infoSection) layout.Dimensions {
	p := th.P
	ws := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		return th.Text(gtx, 12, font.Medium, p.Accent, strings.ToUpper(s.Title))
	}}
	for _, r := range s.Rows {
		r := r
		ws = append(ws, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(190)
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					return th.Text(gtx, 13, font.Normal, p.TextMuted, r[0])
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return th.Paragraph(gtx, 13, p.Text, r[1])
				}),
			)
		})
	}
	return Vert(gtx, 6, ws...)
}
