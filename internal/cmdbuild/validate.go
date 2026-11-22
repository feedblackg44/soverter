package cmdbuild

import (
	"strings"
	"vidpreset/internal/i18n"

	"vidpreset/internal/preset"
)

type Severity int

const (
	Warning Severity = iota // saved and run anyway; shown in the form
	Error                   // the command cannot be built
)

type Issue struct {
	Severity Severity
	Msg      string
}

// Validate returns semantic problems of a structured preset (nil for legacy presets).
// Availability of encoders on this machine is checked elsewhere (package encoders).
func Validate(p preset.Preset) []Issue {
	if !p.Structured() {
		return nil
	}
	v, a := p.Normalized()
	var is []Issue
	errf := func(m string) { is = append(is, Issue{Error, m}) }
	warn := func(m string) { is = append(is, Issue{Warning, m}) }

	if v.Codec == preset.CodecCopy {
		if v.ScaleMode != preset.ScaleNone || v.FPS > 0 || strings.TrimSpace(p.VF) != "" {
			errf(i18n.T("Без перекодирования видеофильтры (размер, fps) недопустимы: выберите кодек."))
		}
		if v.RateMode != preset.RateQuality {
			errf(i18n.T("Режимы «по битрейту» и «по размеру» недоступны без перекодирования видео."))
		}
	} else {
		switch v.RateMode {
		case preset.RateBitrate:
			if v.VideoBitrate <= 0 {
				errf(i18n.T("Укажите видеобитрейт (Мбит/с)."))
			}
		case preset.RateSize:
			if v.TargetSizeMB <= 0 {
				errf(i18n.T("Укажите целевой размер файла (МБ)."))
			}
		}
		switch v.ScaleMode {
		case preset.ScaleKeep:
			if v.Height <= 0 {
				errf(i18n.T("Для «сохранить пропорции» укажите высоту."))
			}
		case preset.ScaleStretch, preset.ScaleFit:
			if v.Width <= 0 || v.Height <= 0 {
				errf(i18n.T("Укажите ширину и высоту кадра."))
			}
		}
	}

	if a.Mode == preset.AudioCopy && (a.VolumeDB != 0 || strings.TrimSpace(p.AF) != "") {
		errf(i18n.T("Изменение громкости и аудиофильтры требуют перекодирования звука: выберите AAC или Opus."))
	}
	if a.Mode == preset.AudioRemove && a.VolumeDB != 0 {
		warn(i18n.T("Звук убирается, громкость не будет применена."))
	}

	// container compatibility
	ext := strings.TrimPrefix(strings.ToLower(p.Ext), ".")
	switch ext {
	case "webm":
		if v.Codec == preset.CodecH264 || v.Codec == preset.CodecH265 {
			warn(i18n.T("WebM поддерживает только VP8/VP9/AV1: H.264 и H.265 в нём работать не будут. Выберите AV1 или контейнер mp4/mkv."))
		}
		if v.Codec == preset.CodecCopy {
			warn(i18n.T("В WebM допустимы только VP8/VP9/AV1: копирование видео сработает не для любого источника."))
		}
		switch a.Mode {
		case preset.AudioAAC:
			warn(i18n.T("WebM требует звук Opus или Vorbis: AAC не поддерживается. Выберите Opus."))
		case preset.AudioCopy:
			warn(i18n.T("В WebM допустим только Opus/Vorbis: копирование звука сработает не для любого источника."))
		}
	case "mp4", "m4v", "mov":
		if a.Mode == preset.AudioOpus {
			warn(i18n.T("Opus в MP4/MOV поддерживается не всеми плеерами; надёжнее AAC."))
		}
		if ext == "mov" && v.Codec == preset.CodecAV1 {
			warn(i18n.T("AV1 в MOV поддерживается плохо; лучше mp4 или mkv."))
		}
	case "mp3", "m4a", "wav", "flac", "ogg", "opus":
		warn(i18n.T("Это аудио-контейнер: для видео выберите mp4/mkv/webm."))
	}
	return is
}

// Availability tells whether an encoder works on this machine (implemented by encoders.Report).
type Availability interface {
	OK(name string) bool
	Why(name string) string
}

// CheckAvailability returns warnings for encoders the preset needs but this machine cannot run.
// A preset with an unavailable encoder still opens and saves; it just cannot run until another is chosen.
func CheckAvailability(p preset.Preset, av Availability) []Issue {
	var is []Issue
	if av == nil {
		return nil
	}
	for _, n := range Encoders(p) {
		if !av.OK(n) {
			is = append(is, Issue{Warning, i18n.T("Энкодер ") + n + i18n.T(" недоступен на этой машине: ") + av.Why(n) + i18n.T(". Выберите другой энкодер, иначе пресет не запустится.")})
		}
	}
	return is
}
