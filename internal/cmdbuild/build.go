// Package cmdbuild turns a preset into ffmpeg arguments. It is pure (no UI, no processes, no files),
// so every encoder family and rate mode can be unit-tested.
package cmdbuild

import (
	"fmt"
	"os"
	"soverter/internal/i18n"
	"strconv"
	"strings"

	"soverter/internal/preset"
)

// Opts carries run-time facts the preset itself cannot know.
type Opts struct {
	SrcExt    string // source extension (".mp4") for ext = "same"
	VideoKbps int    // rate_mode = size: video bitrate computed from the target size
	Pass      int    // 0 = single pass; 1 or 2 = pass of a two-pass encode
	PassLog   string // -passlogfile prefix (two-pass)
	Preview   bool   // size mode without a computed bitrate: show a placeholder instead of failing
}

// EncoderName returns the ffmpeg encoder for a codec and family ("" for copy).
func EncoderName(codec, family string) string {
	if codec == preset.CodecCopy {
		return ""
	}
	switch family {
	case preset.EncNVENC:
		return map[string]string{preset.CodecH264: "h264_nvenc", preset.CodecH265: "hevc_nvenc", preset.CodecAV1: "av1_nvenc"}[codec]
	case preset.EncAMF:
		return map[string]string{preset.CodecH264: "h264_amf", preset.CodecH265: "hevc_amf", preset.CodecAV1: "av1_amf"}[codec]
	case preset.EncQSV:
		return map[string]string{preset.CodecH264: "h264_qsv", preset.CodecH265: "hevc_qsv", preset.CodecAV1: "av1_qsv"}[codec]
	}
	return map[string]string{preset.CodecH264: "libx264", preset.CodecH265: "libx265", preset.CodecAV1: "libsvtav1"}[codec]
}

// ResolveExt returns the output extension without a dot.
func ResolveExt(p preset.Preset, srcExt string) string {
	ext := strings.TrimPrefix(p.Ext, ".")
	if ext == "same" {
		return strings.TrimPrefix(srcExt, ".")
	}
	return ext
}

// Encoders lists the encoders a structured preset needs (for availability checks).
func Encoders(p preset.Preset) []string {
	if !p.Structured() {
		return nil
	}
	v, a := p.Normalized()
	var r []string
	if e := EncoderName(v.Codec, v.Encoder); e != "" {
		r = append(r, e)
	}
	switch a.Mode {
	case preset.AudioAAC:
		r = append(r, "aac")
	case preset.AudioOpus:
		r = append(r, "libopus")
	}
	return r
}

// NeedsTwoPass reports whether the preset should be encoded in two passes (size mode on libx264/libx265).
func NeedsTwoPass(p preset.Preset) bool {
	if !p.Structured() {
		return false
	}
	v, _ := p.Normalized()
	return v.RateMode == preset.RateSize && v.Encoder == preset.EncCPU &&
		(v.Codec == preset.CodecH264 || v.Codec == preset.CodecH265)
}

// NeedsSize reports whether the video bitrate must be computed from the file at run time.
func NeedsSize(p preset.Preset) bool {
	if !p.Structured() {
		return false
	}
	v, _ := p.Normalized()
	return v.Codec != preset.CodecCopy && v.RateMode == preset.RateSize
}

// Args builds the full ffmpeg argument list (without the executable).
func Args(p preset.Preset, in, out string, o Opts) ([]string, error) {
	if !p.Structured() {
		return legacyArgs(p, in, out), nil
	}
	for _, is := range Validate(p) {
		if is.Severity == Error {
			return nil, fmt.Errorf("%s", is.Msg)
		}
	}
	v, a := p.Normalized()
	a2 := []string{"-hide_banner", "-n", "-i", in}

	if v.Codec != preset.CodecCopy {
		if vf := joinFilters(videoFilters(v), p.VF); vf != "" {
			a2 = append(a2, "-vf", vf)
		}
	}
	if a.Mode != preset.AudioCopy && a.Mode != preset.AudioRemove {
		var f []string
		if a.VolumeDB != 0 {
			f = append(f, "volume="+fnum(a.VolumeDB)+"dB")
		}
		if af := joinFilters(strings.Join(f, ","), p.AF); af != "" {
			a2 = append(a2, "-af", af)
		}
	}

	// video codec
	if v.Codec == preset.CodecCopy {
		a2 = append(a2, "-c:v", "copy")
	} else {
		ra, err := videoArgs(v, o)
		if err != nil {
			return nil, err
		}
		a2 = append(a2, ra...)
		if v.Codec == preset.CodecH265 && hvc1Container(ResolveExt(p, orDefault(o.SrcExt, ".mp4"))) {
			a2 = append(a2, "-tag:v", "hvc1")
		}
	}
	if o.Pass != 0 {
		a2 = append(a2, "-pass", strconv.Itoa(o.Pass))
		if o.PassLog != "" {
			a2 = append(a2, "-passlogfile", o.PassLog)
		}
	}

	// audio
	switch {
	case o.Pass == 1 || a.Mode == preset.AudioRemove:
		a2 = append(a2, "-an")
	case a.Mode == preset.AudioCopy:
		a2 = append(a2, "-c:a", "copy")
	case a.Mode == preset.AudioAAC:
		a2 = append(a2, "-c:a", "aac", "-b:a", orDefault(a.Bitrate, "192k"))
	case a.Mode == preset.AudioOpus:
		a2 = append(a2, "-c:a", "libopus", "-b:a", orDefault(a.Bitrate, "128k"))
	}

	// Extra arguments go into both passes: encoder options (e.g. -tune) must match; muxer options
	// such as -movflags are harmless on the null muxer.
	a2 = append(a2, p.Args...)
	if o.Pass == 1 {
		return append(a2, "-f", "null", os.DevNull), nil
	}
	return append(a2, out), nil
}

func legacyArgs(p preset.Preset, in, out string) []string {
	a := []string{"-hide_banner", "-n", "-i", in}
	if p.VF != "" {
		a = append(a, "-vf", p.VF)
	}
	if p.AF != "" {
		a = append(a, "-af", p.AF)
	}
	a = append(a, p.Args...)
	return append(a, out)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func hvc1Container(ext string) bool { return ext == "mp4" || ext == "mov" || ext == "m4v" }

// fnum formats a float without trailing zeros.
func fnum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func joinFilters(parts ...string) string {
	var r []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			r = append(r, p)
		}
	}
	return strings.Join(r, ",")
}

// videoFilters builds the generated part of the -vf chain: fps first, then scaling.
func videoFilters(v preset.Video) string {
	var f []string
	if v.FPS > 0 {
		f = append(f, "fps="+fnum(v.FPS))
	}
	w, h := strconv.Itoa(v.Width), strconv.Itoa(v.Height)
	switch v.ScaleMode {
	case preset.ScaleKeep:
		f = append(f, "scale=-2:"+h)
	case preset.ScaleStretch:
		f = append(f, "scale="+w+":"+h, "setsar=1")
	case preset.ScaleFit:
		f = append(f, "scale="+w+":"+h+":force_original_aspect_ratio=decrease",
			"pad="+w+":"+h+":(ow-iw)/2:(oh-ih)/2", "setsar=1")
	}
	return strings.Join(f, ",")
}

// videoArgs returns "-c:v <enc>" plus rate-control and speed arguments.
func videoArgs(v preset.Video, o Opts) ([]string, error) {
	enc := EncoderName(v.Codec, v.Encoder)
	if enc == "" {
		return nil, fmt.Errorf(i18n.T("нет энкодера для %s/%s"), v.Codec, v.Encoder)
	}
	args := []string{"-c:v", enc}
	q := strconv.Itoa(v.Quality)
	switch v.RateMode {
	case preset.RateQuality:
		args = append(args, qualityArgs(v, q)...)
	case preset.RateBitrate:
		bv := fnum(v.VideoBitrate) + "M"
		args = append(args, bitrateArgs(v.Encoder, bv, fnum(v.VideoBitrate*1.5)+"M")...)
	case preset.RateSize:
		kbps := o.VideoKbps
		switch {
		case kbps > 0:
			if v.Encoder != preset.EncCPU { // hardware VBR overshoots more: keep a margin
				kbps = int(float64(kbps) * 0.95)
			}
			mr := kbps
			if v.Encoder != preset.EncCPU {
				mr = kbps * 12 / 10
			}
			args = append(args, bitrateArgs(v.Encoder, fmt.Sprintf("%dk", kbps), fmt.Sprintf("%dk", mr))...)
		case o.Preview:
			args = append(args, bitrateArgs(v.Encoder, i18n.T("<рассчитывается при запуске>k"), "<…>k")...)
		default:
			return nil, fmt.Errorf("%s", i18n.T("для режима «по размеру» не рассчитан видеобитрейт"))
		}
	}
	return append(args, speedArgs(v)...), nil
}

func qualityArgs(v preset.Video, q string) []string {
	switch v.Encoder {
	case preset.EncNVENC:
		return []string{"-rc", "vbr", "-cq", q, "-b:v", "0"}
	case preset.EncQSV:
		return []string{"-global_quality", q}
	case preset.EncAMF:
		a := []string{"-rc", "cqp", "-qp_i", q, "-qp_p", q}
		if v.Codec == preset.CodecH264 {
			a = append(a, "-qp_b", q)
		}
		return a
	}
	return []string{"-crf", q}
}

func bitrateArgs(family, bv, maxrate string) []string {
	switch family {
	case preset.EncNVENC:
		return []string{"-rc", "vbr", "-b:v", bv, "-maxrate", maxrate}
	case preset.EncAMF:
		return []string{"-rc", "vbr_peak", "-b:v", bv, "-maxrate", maxrate}
	case preset.EncQSV:
		return []string{"-b:v", bv, "-maxrate", maxrate}
	}
	return []string{"-b:v", bv}
}

var speedTables = map[string]map[string]string{
	"x264":   {"fastest": "ultrafast", "fast": "veryfast", "medium": "medium", "slow": "slow", "slowest": "veryslow"},
	"svtav1": {"fastest": "12", "fast": "10", "medium": "8", "slow": "5", "slowest": "3"},
	"nvenc":  {"fastest": "p1", "fast": "p3", "medium": "p4", "slow": "p5", "slowest": "p7"},
	"qsv":    {"fastest": "veryfast", "fast": "fast", "medium": "medium", "slow": "slow", "slowest": "veryslow"},
	"amf":    {"fastest": "speed", "fast": "speed", "medium": "balanced", "slow": "quality", "slowest": "quality"},
}

// SpeedValue translates a unified speed name into the encoder's own value; unknown names pass through.
func SpeedValue(codec, family, speed string) string {
	key := "x264"
	switch family {
	case preset.EncNVENC:
		key = "nvenc"
	case preset.EncQSV:
		key = "qsv"
	case preset.EncAMF:
		key = "amf"
	default:
		if codec == preset.CodecAV1 {
			key = "svtav1"
		}
	}
	if s, ok := speedTables[key][speed]; ok {
		return s
	}
	return speed
}

func speedArgs(v preset.Video) []string {
	if v.Speed == "" {
		return nil
	}
	val := SpeedValue(v.Codec, v.Encoder, v.Speed)
	if v.Encoder == preset.EncAMF {
		return []string{"-quality", val}
	}
	return []string{"-preset", val}
}
