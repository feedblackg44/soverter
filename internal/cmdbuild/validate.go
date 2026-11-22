package cmdbuild

import (
	"strings"

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
			errf("Без перекодирования видеофильтры (размер, fps) недопустимы: выберите кодек.")
		}
		if v.RateMode != preset.RateQuality {
			errf("Режимы «по битрейту» и «по размеру» недоступны без перекодирования видео.")
		}
	} else {
		switch v.RateMode {
		case preset.RateBitrate:
			if v.VideoBitrate <= 0 {
				errf("Укажите видеобитрейт (Мбит/с).")
			}
		case preset.RateSize:
			if v.TargetSizeMB <= 0 {
				errf("Укажите целевой размер файла (МБ).")
			}
		}
		switch v.ScaleMode {
		case preset.ScaleKeep:
			if v.Height <= 0 {
				errf("Для «сохранить пропорции» укажите высоту.")
			}
		case preset.ScaleStretch, preset.ScaleFit:
			if v.Width <= 0 || v.Height <= 0 {
				errf("Укажите ширину и высоту кадра.")
			}
		}
	}

	if a.Mode == preset.AudioCopy && (a.VolumeDB != 0 || strings.TrimSpace(p.AF) != "") {
		errf("Изменение громкости и аудиофильтры требуют перекодирования звука: выберите AAC или Opus.")
	}
	if a.Mode == preset.AudioRemove && a.VolumeDB != 0 {
		warn("Звук убирается, громкость не будет применена.")
	}

	// container compatibility
	ext := strings.TrimPrefix(strings.ToLower(p.Ext), ".")
	switch ext {
	case "webm":
		if v.Codec == preset.CodecH264 || v.Codec == preset.CodecH265 {
			warn("WebM поддерживает только VP8/VP9/AV1: H.264 и H.265 в нём работать не будут. Выберите AV1 или контейнер mp4/mkv.")
		}
		if v.Codec == preset.CodecCopy {
			warn("В WebM допустимы только VP8/VP9/AV1: копирование видео сработает не для любого источника.")
		}
		switch a.Mode {
		case preset.AudioAAC:
			warn("WebM требует звук Opus или Vorbis: AAC не поддерживается. Выберите Opus.")
		case preset.AudioCopy:
			warn("В WebM допустим только Opus/Vorbis: копирование звука сработает не для любого источника.")
		}
	case "mp4", "m4v", "mov":
		if a.Mode == preset.AudioOpus {
			warn("Opus в MP4/MOV поддерживается не всеми плеерами; надёжнее AAC.")
		}
		if ext == "mov" && v.Codec == preset.CodecAV1 {
			warn("AV1 в MOV поддерживается плохо; лучше mp4 или mkv.")
		}
	case "mp3", "m4a", "wav", "flac", "ogg", "opus":
		warn("Это аудио-контейнер: для видео выберите mp4/mkv/webm.")
	}
	return is
}
