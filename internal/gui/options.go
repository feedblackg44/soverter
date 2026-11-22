package gui

import (
	"strconv"
	"strings"
	"vidpreset/internal/i18n"

	"vidpreset/internal/preset"
)

func labelOf(os []Opt, val string) string {
	for _, o := range os {
		if o.Val == val {
			return o.Label
		}
	}
	return ""
}

func codecOpts() []Opt {
	return []Opt{
		{Val: preset.CodecCopy, Label: i18n.T("Без перекодирования")},
		{Val: preset.CodecH264, Label: "H.264"},
		{Val: preset.CodecH265, Label: "H.265 (HEVC)"},
		{Val: preset.CodecAV1, Label: "AV1"},
	}
}

func familyNames() []Opt {
	return []Opt{
		{Val: preset.EncCPU, Label: "CPU"},
		{Val: preset.EncNVENC, Label: "NVIDIA NVENC"},
		{Val: preset.EncAMF, Label: "AMD AMF"},
		{Val: preset.EncQSV, Label: "Intel QSV"},
	}
}

func rateOpts() []Opt {
	return []Opt{
		{Val: preset.RateQuality, Label: i18n.T("По качеству")},
		{Val: preset.RateBitrate, Label: i18n.T("По битрейту")},
		{Val: preset.RateSize, Label: i18n.T("По размеру файла")},
	}
}

func speedOpts() []Opt {
	return []Opt{
		{Val: "", Label: i18n.T("По умолчанию")},
		{Val: "fastest", Label: i18n.T("Самая быстрая")},
		{Val: "fast", Label: i18n.T("Быстрая")},
		{Val: "medium", Label: i18n.T("Средняя")},
		{Val: "slow", Label: i18n.T("Медленная")},
		{Val: "slowest", Label: i18n.T("Самая медленная")},
	}
}

func scaleModeOpts() []Opt {
	return []Opt{
		{Val: preset.ScaleKeep, Label: i18n.T("Сохранить пропорции")},
		{Val: preset.ScaleStretch, Label: i18n.T("Растянуть")},
		{Val: preset.ScaleFit, Label: i18n.T("Вписать с чёрными полями")},
	}
}

func audioOpts() []Opt {
	return []Opt{
		{Val: preset.AudioCopy, Label: i18n.T("Копировать")},
		{Val: preset.AudioAAC, Label: "AAC"},
		{Val: preset.AudioOpus, Label: "Opus"},
		{Val: preset.AudioRemove, Label: i18n.T("Убрать звук")},
	}
}

func audioBitrates() []Opt {
	return []Opt{
		{Val: "96k", Label: i18n.T("96 к")}, {Val: "128k", Label: i18n.T("128 к")}, {Val: "160k", Label: i18n.T("160 к")},
		{Val: "192k", Label: i18n.T("192 к")}, {Val: "256k", Label: i18n.T("256 к")}, {Val: "320k", Label: i18n.T("320 к")},
	}
}

func fpsOpts() []Opt {
	return []Opt{
		{Val: "", Label: i18n.T("Не менять")}, {Val: "24", Label: "24"}, {Val: "25", Label: "25"},
		{Val: "30", Label: "30"}, {Val: "50", Label: "50"}, {Val: "60", Label: "60"},
	}
}

func extOpts() []Opt {
	return []Opt{
		{Val: "same", Label: i18n.T("Как у источника")}, {Val: "mp4", Label: "mp4"}, {Val: "mkv", Label: "mkv"},
		{Val: "webm", Label: "webm"}, {Val: "mov", Label: "mov"}, {Val: "m4v", Label: "m4v"},
		{Val: "avi", Label: "avi"}, {Val: "mp3", Label: "mp3"}, {Val: "m4a", Label: "m4a"}, {Val: "wav", Label: "wav"},
	}
}

type sizePreset struct {
	val, label string
	w, h       int
}

const (
	sizeNone   = "none"
	sizeCustom = "custom"
)

func hwHint() string {
	return i18n.T("Аппаратное кодирование быстрее, но при том же размере файла качество обычно немного ниже, чем у libx264/libx265.")
}

func sizePresets() []sizePreset {
	return []sizePreset{
		{sizeNone, i18n.T("Не менять"), 0, 0},
		{"720", "720p", 1280, 720},
		{"1080", "1080p", 1920, 1080},
		{"1440", "1440p", 2560, 1440},
		{"2160", "2160p (4K)", 3840, 2160},
		{sizeCustom, i18n.T("Своё разрешение"), 0, 0},
	}
}

func sizeOpts() []Opt {
	var o []Opt
	for _, s := range sizePresets() {
		o = append(o, Opt{Val: s.val, Label: s.label})
	}
	return o
}

// sizeKeyFor picks the size choice matching a preset's scale settings.
func sizeKeyFor(v preset.Video) string {
	if v.ScaleMode == preset.ScaleNone {
		return sizeNone
	}
	for _, s := range sizePresets() {
		if s.w == 0 {
			continue
		}
		if v.ScaleMode == preset.ScaleKeep && v.Height == s.h {
			return s.val
		}
		if v.ScaleMode != preset.ScaleKeep && v.Width == s.w && v.Height == s.h {
			return s.val
		}
	}
	return sizeCustom
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func parseNum(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func numOrEmpty(f float64) string {
	if f == 0 {
		return ""
	}
	return fmtNum(f)
}

func intOrEmpty(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func clonePreset(p preset.Preset) preset.Preset {
	n := p
	n.Args = append([]string(nil), p.Args...)
	if p.Video != nil {
		v := *p.Video
		n.Video = &v
	}
	if p.Audio != nil {
		a := *p.Audio
		n.Audio = &a
	}
	return n
}

// newStructured is the starting point of every new preset: H.264 on CPU by quality, audio copied.
func newStructured(id string) preset.Preset {
	return preset.Preset{
		ID: id, Name: i18n.T("Новый пресет"), Suffix: "new", Ext: "mp4",
		Video: &preset.Video{Codec: preset.CodecH264, Encoder: preset.EncCPU, RateMode: preset.RateQuality, Quality: 20, Speed: "medium"},
		Audio: &preset.Audio{Mode: preset.AudioCopy},
	}
}
