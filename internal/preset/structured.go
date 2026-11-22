package preset

import (
	"fmt"
	"strings"
)

// Structured presets are described by the [preset.video] / [preset.audio] tables and built into an
// ffmpeg command by package cmdbuild. A preset with neither table is a legacy "raw" preset
// (vf / af / args only) and keeps working unchanged.
//
// In a structured preset the existing fields keep a meaning:
//
//	vf    extra video filters, appended to the generated -vf chain
//	af    extra audio filters, appended to the generated -af chain
//	args  extra raw ffmpeg arguments, placed at the end before the output file
type Video struct {
	Codec        string  `toml:"codec"`                             // copy | h264 | h265 | av1
	Encoder      string  `toml:"encoder,omitempty"`                 // cpu (default) | nvenc | amf | qsv
	RateMode     string  `toml:"rate_mode,omitempty"`               // quality (default) | bitrate | size
	Quality      int     `toml:"quality,omitempty,omitzero"`        // unified scale, lower = better (CRF-like), rate_mode=quality
	VideoBitrate float64 `toml:"video_bitrate,omitempty,omitzero"`  // Mbit/s, rate_mode=bitrate
	TargetSizeMB float64 `toml:"target_size_mb,omitempty,omitzero"` // MB, rate_mode=size
	Speed        string  `toml:"speed,omitempty"`                   // fastest | fast | medium | slow | slowest (or a raw encoder preset)
	ScaleMode    string  `toml:"scale_mode,omitempty"`              // none (default) | keep | stretch | fit
	Width        int     `toml:"width,omitempty,omitzero"`
	Height       int     `toml:"height,omitempty,omitzero"`
	FPS          float64 `toml:"fps,omitempty,omitzero"` // 0 = unchanged
}

type Audio struct {
	Mode     string  `toml:"mode"`                         // copy | aac | opus | remove
	Bitrate  string  `toml:"bitrate,omitempty"`            // for aac / opus, e.g. "192k"
	VolumeDB float64 `toml:"volume_db,omitempty,omitzero"` // != 0 requires re-encoding
}

const (
	CodecCopy = "copy"
	CodecH264 = "h264"
	CodecH265 = "h265"
	CodecAV1  = "av1"

	EncCPU   = "cpu"
	EncNVENC = "nvenc"
	EncAMF   = "amf"
	EncQSV   = "qsv"

	RateQuality = "quality"
	RateBitrate = "bitrate"
	RateSize    = "size"

	ScaleNone    = "none"
	ScaleKeep    = "keep"
	ScaleStretch = "stretch"
	ScaleFit     = "fit"

	AudioCopy   = "copy"
	AudioAAC    = "aac"
	AudioOpus   = "opus"
	AudioRemove = "remove"
)

// SpeedLevels are the unified speed names in order from fastest to slowest.
var SpeedLevels = []string{"fastest", "fast", "medium", "slow", "slowest"}

// Structured reports whether the preset is built from the form model.
func (p Preset) Structured() bool { return p.Video != nil || p.Audio != nil }

// Normalized returns copies of Video/Audio with defaults filled in. Safe to call on legacy presets
// (returns zero-value defaults, which no caller should use for building).
func (p Preset) Normalized() (Video, Audio) {
	v := Video{Codec: CodecH264, Encoder: EncCPU, RateMode: RateQuality, ScaleMode: ScaleNone}
	if p.Video != nil {
		v = *p.Video
		if v.Codec == "" {
			v.Codec = CodecH264
		}
		if v.Encoder == "" {
			v.Encoder = EncCPU
		}
		if v.RateMode == "" {
			v.RateMode = RateQuality
		}
		if v.ScaleMode == "" {
			v.ScaleMode = ScaleNone
		}
	}
	a := Audio{Mode: AudioCopy}
	if p.Audio != nil {
		a = *p.Audio
		if a.Mode == "" {
			a.Mode = AudioCopy
		}
	}
	return v, a
}

func oneOf(s string, set ...string) bool {
	for _, x := range set {
		if s == x {
			return true
		}
	}
	return false
}

// validateStructured checks enumerations and ranges only; semantic checks (copy + filters,
// container compatibility) live in cmdbuild.Validate.
func (p Preset) validateStructured() error {
	v, a := p.Normalized()
	bad := func(field, val string) error {
		return fmt.Errorf("preset %s: недопустимое значение %s = %q", p.ID, field, val)
	}
	if !oneOf(v.Codec, CodecCopy, CodecH264, CodecH265, CodecAV1) {
		return bad("video.codec", v.Codec)
	}
	if !oneOf(v.Encoder, EncCPU, EncNVENC, EncAMF, EncQSV) {
		return bad("video.encoder", v.Encoder)
	}
	if !oneOf(v.RateMode, RateQuality, RateBitrate, RateSize) {
		return bad("video.rate_mode", v.RateMode)
	}
	if !oneOf(v.ScaleMode, ScaleNone, ScaleKeep, ScaleStretch, ScaleFit) {
		return bad("video.scale_mode", v.ScaleMode)
	}
	if !oneOf(a.Mode, AudioCopy, AudioAAC, AudioOpus, AudioRemove) {
		return bad("audio.mode", a.Mode)
	}
	if v.Quality < 0 || v.Quality > 63 {
		return fmt.Errorf("preset %s: quality должно быть 0..63", p.ID)
	}
	if v.VideoBitrate < 0 || v.TargetSizeMB < 0 || v.FPS < 0 || v.Width < 0 || v.Height < 0 {
		return fmt.Errorf("preset %s: отрицательные значения недопустимы", p.ID)
	}
	if v.Speed != "" && strings.ContainsAny(v.Speed, " \t") {
		return bad("video.speed", v.Speed)
	}
	return nil
}
