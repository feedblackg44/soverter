package gui

import (
	"strconv"
	"strings"

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

var (
	codecOpts = []Opt{
		{Val: preset.CodecCopy, Label: "Без перекодирования"},
		{Val: preset.CodecH264, Label: "H.264"},
		{Val: preset.CodecH265, Label: "H.265 (HEVC)"},
		{Val: preset.CodecAV1, Label: "AV1"},
	}
	familyNames = []Opt{
		{Val: preset.EncCPU, Label: "CPU"},
		{Val: preset.EncNVENC, Label: "NVIDIA NVENC"},
		{Val: preset.EncAMF, Label: "AMD AMF"},
		{Val: preset.EncQSV, Label: "Intel QSV"},
	}
	rateOpts = []Opt{
		{Val: preset.RateQuality, Label: "По качеству"},
		{Val: preset.RateBitrate, Label: "По битрейту"},
		{Val: preset.RateSize, Label: "По размеру файла"},
	}
	speedOpts = []Opt{
		{Val: "", Label: "По умолчанию"},
		{Val: "fastest", Label: "Самая быстрая"},
		{Val: "fast", Label: "Быстрая"},
		{Val: "medium", Label: "Средняя"},
		{Val: "slow", Label: "Медленная"},
		{Val: "slowest", Label: "Самая медленная"},
	}
	scaleModeOpts = []Opt{
		{Val: preset.ScaleKeep, Label: "Сохранить пропорции"},
		{Val: preset.ScaleStretch, Label: "Растянуть"},
		{Val: preset.ScaleFit, Label: "Вписать с чёрными полями"},
	}
	audioOpts = []Opt{
		{Val: preset.AudioCopy, Label: "Копировать"},
		{Val: preset.AudioAAC, Label: "AAC"},
		{Val: preset.AudioOpus, Label: "Opus"},
		{Val: preset.AudioRemove, Label: "Убрать звук"},
	}
	audioBitrates = []Opt{
		{Val: "96k", Label: "96 к"}, {Val: "128k", Label: "128 к"}, {Val: "160k", Label: "160 к"},
		{Val: "192k", Label: "192 к"}, {Val: "256k", Label: "256 к"}, {Val: "320k", Label: "320 к"},
	}
	fpsOpts = []Opt{
		{Val: "", Label: "Не менять"}, {Val: "24", Label: "24"}, {Val: "25", Label: "25"},
		{Val: "30", Label: "30"}, {Val: "50", Label: "50"}, {Val: "60", Label: "60"},
	}
	extOpts = []Opt{
		{Val: "same", Label: "Как у источника"}, {Val: "mp4", Label: "mp4"}, {Val: "mkv", Label: "mkv"},
		{Val: "webm", Label: "webm"}, {Val: "mov", Label: "mov"}, {Val: "m4v", Label: "m4v"},
		{Val: "avi", Label: "avi"}, {Val: "mp3", Label: "mp3"}, {Val: "m4a", Label: "m4a"}, {Val: "wav", Label: "wav"},
	}
)

type sizePreset struct {
	val, label string
	w, h       int
}

const (
	sizeNone   = "none"
	sizeCustom = "custom"
	hwHint     = "Аппаратное кодирование быстрее, но при том же размере файла качество обычно немного ниже, чем у libx264/libx265."
)

var sizePresets = []sizePreset{
	{sizeNone, "Не менять", 0, 0},
	{"720", "720p", 1280, 720},
	{"1080", "1080p", 1920, 1080},
	{"1440", "1440p", 2560, 1440},
	{"2160", "2160p (4K)", 3840, 2160},
	{sizeCustom, "Своё…", 0, 0},
}

func sizeOpts() []Opt {
	var o []Opt
	for _, s := range sizePresets {
		o = append(o, Opt{Val: s.val, Label: s.label})
	}
	return o
}

// sizeKeyFor picks the size choice matching a preset's scale settings.
func sizeKeyFor(v preset.Video) string {
	if v.ScaleMode == preset.ScaleNone {
		return sizeNone
	}
	for _, s := range sizePresets {
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
		ID: id, Name: "Новый пресет", Suffix: "new", Ext: "mp4",
		Video: &preset.Video{Codec: preset.CodecH264, Encoder: preset.EncCPU, RateMode: preset.RateQuality, Quality: 20, Speed: "medium"},
		Audio: &preset.Audio{Mode: preset.AudioCopy},
	}
}
