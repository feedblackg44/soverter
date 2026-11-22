// Package ffimport turns a pasted ffmpeg command line into a structured preset.
// It is pure: text in, preset + report out. Anything it does not understand is kept as raw arguments.
package ffimport

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"vidpreset/internal/i18n"

	"vidpreset/internal/cmdbuild"
	"vidpreset/internal/preset"
)

// Result is the outcome of Parse.
type Result struct {
	Preset     preset.Preset // Video/Audio filled; VF/AF/Args hold the unrecognized remainder; Ext from the output file
	Recognized []string      // what was mapped onto form fields, human readable
	Ignored    []string      // dropped on purpose (ffmpeg, -i <input>, output file, noise flags)
	Extra      []string      // unrecognized arguments kept raw (same content as Preset.Args plus filters)
	Warnings   []string
}

var (
	noValueFlags = map[string]bool{
		"-y": true, "-n": true, "-hide_banner": true, "-nostdin": true, "-stats": true, "-nostats": true,
		"-an": true, "-vn": true, "-sn": true, "-dn": true, "-shortest": true,
	}
	noiseFlags    = map[string]bool{"-y": true, "-n": true, "-hide_banner": true, "-nostdin": true, "-stats": true, "-nostats": true}
	noiseWithVals = map[string]bool{"-loglevel": true, "-v": true}
	numRe         = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
)

// joinContinuations removes shell line continuations (PowerShell `, cmd ^, bash \) and newlines.
func joinContinuations(s string) string {
	for _, c := range []string{"`", "^", "\\"} {
		for _, nl := range []string{"\r\n", "\n"} {
			s = strings.ReplaceAll(s, c+nl, " ")
		}
	}
	return strings.NewReplacer("\r\n", " ", "\n", " ").Replace(s)
}

// encoderMap maps ffmpeg encoder names to (codec, family).
var encoderMap = map[string][2]string{
	"libx264": {"h264", "cpu"}, "h264": {"h264", "cpu"},
	"libx265": {"h265", "cpu"}, "hevc": {"h265", "cpu"}, "h265": {"h265", "cpu"},
	"libsvtav1":  {"av1", "cpu"},
	"h264_nvenc": {"h264", "nvenc"}, "hevc_nvenc": {"h265", "nvenc"}, "av1_nvenc": {"av1", "nvenc"},
	"h264_amf": {"h264", "amf"}, "hevc_amf": {"h265", "amf"}, "av1_amf": {"av1", "amf"},
	"h264_qsv": {"h264", "qsv"}, "hevc_qsv": {"h265", "qsv"}, "av1_qsv": {"av1", "qsv"},
}

type parser struct {
	res Result
	v   preset.Video
	a   preset.Audio

	vfLeft, afLeft []string
	extra          []string

	haveVCodec, haveACodec bool
	crf, cq, gq            string // quality flags seen
	qp                     map[string]string
	bv, maxrate, preset    string
	qualityFlag            string // "-quality" (amf) value
	rc                     string
	fpsOpt                 string
	tagHvc1                bool
	audioBitrate           string
	outName                string
}

func (p *parser) recognize(format string, a ...any) {
	p.res.Recognized = append(p.res.Recognized, fmt.Sprintf(format, a...))
}
func (p *parser) ignore(format string, a ...any) {
	p.res.Ignored = append(p.res.Ignored, fmt.Sprintf(format, a...))
}
func (p *parser) warn(format string, a ...any) {
	p.res.Warnings = append(p.res.Warnings, fmt.Sprintf(format, a...))
}
func (p *parser) keep(args ...string) { p.extra = append(p.extra, args...) }

// Parse analyses an ffmpeg command line.
func Parse(cmdline string) (Result, error) {
	toks, err := preset.SplitArgs(joinContinuations(cmdline))
	if err != nil {
		return Result{}, fmt.Errorf(i18n.T("не удалось разобрать команду: %w"), err)
	}
	if len(toks) == 0 {
		return Result{}, fmt.Errorf("%s", i18n.T("команда пустая"))
	}
	p := &parser{qp: map[string]string{}}
	p.v = preset.Video{Codec: preset.CodecH264, Encoder: preset.EncCPU, RateMode: preset.RateQuality, ScaleMode: preset.ScaleNone}
	p.a = preset.Audio{Mode: preset.AudioCopy}

	i := 0
	base := strings.ToLower(strings.TrimSuffix(strings.ToLower(filepath.Base(strings.ReplaceAll(toks[0], "\\", "/"))), ".exe"))
	if base == "ffmpeg" {
		p.ignore("%s", toks[0])
		i = 1
	}

	sawInput := false
	var positionals []string
	for ; i < len(toks); i++ {
		t := toks[i]
		next := func() (string, bool) {
			if i+1 < len(toks) {
				i++
				return toks[i], true
			}
			return "", false
		}
		if !strings.HasPrefix(t, "-") || numRe.MatchString(t) {
			positionals = append(positionals, t)
			continue
		}
		if noiseFlags[t] {
			p.ignore("%s", t)
			continue
		}
		if noiseWithVals[t] {
			if v, ok := next(); ok {
				p.ignore("%s %s", t, v)
			}
			continue
		}
		switch t {
		case "-i":
			v, _ := next()
			if sawInput {
				p.warn(i18n.T("Несколько входных файлов (-i) форма не поддерживает: %s оставлен в дополнительных аргументах и, вероятно, не заработает."), v)
				p.keep(t, v)
			} else {
				sawInput = true
				p.ignore("-i %s", v)
			}
		case "-c:v", "-vcodec", "-codec:v":
			v, _ := next()
			p.videoCodec(v)
		case "-c:a", "-acodec", "-codec:a":
			v, _ := next()
			p.audioCodec(v)
		case "-c", "-codec":
			v, _ := next()
			if v == "copy" {
				p.videoCodec("copy")
				p.audioCodec("copy")
			} else {
				p.warn(i18n.T("%s %s применяется ко всем потокам; оставлено в дополнительных аргументах."), t, v)
				p.keep(t, v)
			}
		case "-crf":
			p.crf, _ = next()
		case "-cq":
			p.cq, _ = next()
		case "-global_quality":
			p.gq, _ = next()
		case "-qp_i", "-qp_p", "-qp_b":
			p.qp[t], _ = next()
		case "-b:v", "-vb":
			p.bv, _ = next()
		case "-maxrate":
			p.maxrate, _ = next()
		case "-preset":
			p.preset, _ = next()
		case "-quality":
			p.qualityFlag, _ = next()
		case "-rc":
			p.rc, _ = next()
		case "-r":
			p.fpsOpt, _ = next()
		case "-tag:v":
			v, _ := next()
			if v == "hvc1" {
				p.tagHvc1 = true
			} else {
				p.keep(t, v)
			}
		case "-b:a", "-ab":
			p.audioBitrate, _ = next()
		case "-an":
			p.a.Mode = preset.AudioRemove
			p.haveACodec = true
			p.recognize("%s", i18n.T("Звук: убрать (-an)"))
		case "-vf", "-filter:v":
			v, _ := next()
			p.videoFilters(v)
		case "-af", "-filter:a":
			v, _ := next()
			p.audioFilters(v)
		default:
			if !sawInput {
				// input options (-hwaccel, -ss before -i, ...) cannot be placed before -i by the builder
				v := ""
				if !noValueFlags[t] && i+1 < len(toks) && !strings.HasPrefix(toks[i+1], "-") {
					v, _ = next()
				}
				p.warn(i18n.T("Входная опция %s %s стоит до -i и не поддерживается формой: отброшена."), t, v)
				p.ignore("%s %s", t, v)
				continue
			}
			if noValueFlags[t] {
				p.keep(t)
				continue
			}
			p.keep(t)
			if i+1 < len(toks) && (!strings.HasPrefix(toks[i+1], "-") || numRe.MatchString(toks[i+1])) && !isLastPositional(toks, i+1) {
				i++
				p.keep(toks[i])
			}
		}
	}

	// output file = last positional; extra positionals are kept
	if n := len(positionals); n > 0 {
		p.outName = positionals[n-1]
		p.ignore(i18n.T("выходной файл %s"), p.outName)
		for _, x := range positionals[:n-1] {
			p.warn(i18n.T("Неопознанный аргумент %q оставлен в дополнительных."), x)
			p.keep(x)
		}
	}
	return p.finish(), nil
}

// isLastPositional reports whether toks[idx] is the final token and not an option (likely the output file).
func isLastPositional(toks []string, idx int) bool {
	return idx == len(toks)-1 && !strings.HasPrefix(toks[idx], "-")
}

func (p *parser) videoCodec(v string) {
	p.haveVCodec = true
	if v == "copy" {
		p.v.Codec = preset.CodecCopy
		p.recognize("%s", i18n.T("Видео: без перекодирования (copy)"))
		return
	}
	if m, ok := encoderMap[v]; ok {
		p.v.Codec, p.v.Encoder = m[0], m[1]
		return
	}
	p.warn(i18n.T("Видеоэнкодер %s форма не поддерживает: он оставлен в дополнительных аргументах и перекроет выбор кодека."), v)
	p.keep("-c:v", v)
}

func (p *parser) audioCodec(v string) {
	p.haveACodec = true
	switch v {
	case "copy":
		p.a.Mode = preset.AudioCopy
		p.recognize("%s", i18n.T("Звук: копировать"))
	case "aac", "libfdk_aac", "aac_at":
		p.a.Mode = preset.AudioAAC
	case "libopus", "opus":
		p.a.Mode = preset.AudioOpus
	default:
		p.warn(i18n.T("Аудиокодек %s форма не поддерживает: он оставлен в дополнительных аргументах."), v)
		p.keep("-c:a", v)
	}
}

// splitFilters splits a filter chain on top-level commas (not inside parentheses or quotes).
func splitFilters(s string) []string {
	var out []string
	depth, start := 0, 0
	var quote rune
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '(' || r == '[':
			depth++
		case r == ')' || r == ']':
			depth--
		case r == ',' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

var (
	scaleKeepRe = regexp.MustCompile(`^scale=-2:(\d+)$`)
	scaleWHRe   = regexp.MustCompile(`^scale=(\d+):(\d+)$`)
	scaleFitRe  = regexp.MustCompile(`^scale=(\d+):(\d+):force_original_aspect_ratio=decrease$`)
	padRe       = regexp.MustCompile(`^pad=(\d+):(\d+):\(ow-iw\)/2:\(oh-ih\)/2$`)
	fpsRe       = regexp.MustCompile(`^fps=(\d+(\.\d+)?)$`)
	volumeRe    = regexp.MustCompile(`^volume=(-?\d+(\.\d+)?)dB$`)
	setsarOne   = map[string]bool{"setsar=1": true, "setsar=1/1": true, "setsar=sar=1": true}
)

func (p *parser) videoFilters(chain string) {
	fs := splitFilters(chain)
	for i := 0; i < len(fs); i++ {
		f := strings.TrimSpace(fs[i])
		peek := func(k int) string {
			if i+k < len(fs) {
				return strings.TrimSpace(fs[i+k])
			}
			return ""
		}
		switch {
		case scaleFitRe.MatchString(f) && padRe.MatchString(peek(1)):
			m := scaleFitRe.FindStringSubmatch(f)
			pm := padRe.FindStringSubmatch(peek(1))
			if m[1] == pm[1] && m[2] == pm[2] {
				p.v.ScaleMode, p.v.Width, p.v.Height = preset.ScaleFit, atoi(m[1]), atoi(m[2])
				p.recognize(i18n.T("Размер: вписать с чёрными полями %s×%s"), m[1], m[2])
				i++
				if setsarOne[peek(1)] {
					i++
				}
				continue
			}
			p.vfLeft = append(p.vfLeft, f)
		case scaleKeepRe.MatchString(f):
			h := scaleKeepRe.FindStringSubmatch(f)[1]
			p.v.ScaleMode, p.v.Height = preset.ScaleKeep, atoi(h)
			p.recognize(i18n.T("Размер: сохранить пропорции, высота %s"), h)
		case scaleWHRe.MatchString(f):
			m := scaleWHRe.FindStringSubmatch(f)
			p.v.ScaleMode, p.v.Width, p.v.Height = preset.ScaleStretch, atoi(m[1]), atoi(m[2])
			p.recognize(i18n.T("Размер: растянуть до %s×%s"), m[1], m[2])
			if setsarOne[peek(1)] {
				i++
			} else {
				p.warn(i18n.T("После scale=%s:%s нет setsar=1: форма добавит его (так пропорции пикселя будут корректны)."), m[1], m[2])
			}
		case fpsRe.MatchString(f):
			p.v.FPS = atof(fpsRe.FindStringSubmatch(f)[1])
			p.recognize(i18n.T("Частота кадров: %s fps"), fpsRe.FindStringSubmatch(f)[1])
		default:
			p.vfLeft = append(p.vfLeft, f)
		}
	}
}

func (p *parser) audioFilters(chain string) {
	for _, f := range splitFilters(chain) {
		f = strings.TrimSpace(f)
		if m := volumeRe.FindStringSubmatch(f); m != nil {
			p.a.VolumeDB = atof(m[1])
			p.recognize(i18n.T("Громкость: %s дБ"), m[1])
			continue
		}
		p.afLeft = append(p.afLeft, f)
	}
}

func atoi(s string) int     { n, _ := strconv.Atoi(s); return n }
func atof(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }

// bitrateMbit parses "5M", "5000k", "5000000" into Mbit/s.
func bitrateMbit(s string) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := 1e-6
	switch {
	case strings.HasSuffix(s, "m"):
		mult, s = 1, strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		mult, s = 1e-3, strings.TrimSuffix(s, "k")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return math.Round(f*mult*1000) / 1000, true
}

func (p *parser) finish() Result {
	v, a := p.v, p.a
	family := v.Encoder

	if !p.haveVCodec {
		p.warn("%s", i18n.T("Видеокодек (-c:v) не указан: выбран H.264 на CPU."))
	} else if v.Codec != preset.CodecCopy {
		p.recognize(i18n.T("Видео: %s, энкодер %s"), map[string]string{"h264": "H.264", "h265": "H.265", "av1": "AV1"}[v.Codec], map[string]string{"cpu": "CPU", "nvenc": "NVIDIA NVENC", "amf": "AMD AMF", "qsv": "Intel QSV"}[family])
	}

	// quality / bitrate
	qv := ""
	switch {
	case p.cq != "":
		qv = p.cq
	case p.crf != "":
		qv = p.crf
	case p.gq != "":
		qv = p.gq
	}
	if qv == "" && family == preset.EncAMF && len(p.qp) > 0 {
		same := ""
		ok := true
		for _, x := range p.qp {
			if same == "" {
				same = x
			} else if same != x {
				ok = false
			}
		}
		if ok {
			qv = same
			p.qp = map[string]string{}
		}
	}
	if p.bv != "" && p.bv != "0" && qv == "" {
		if m, ok := bitrateMbit(p.bv); ok {
			v.RateMode, v.VideoBitrate = preset.RateBitrate, m
			p.recognize(i18n.T("Режим: по битрейту, %g Мбит/с"), m)
			if p.maxrate != "" {
				if mr, ok := bitrateMbit(p.maxrate); ok && math.Abs(mr-m*1.5) < 0.01 {
					p.recognize(i18n.T("Максимальный битрейт: %g Мбит/с (форма задаёт 1,5× автоматически)"), mr)
					p.maxrate = ""
				}
			}
		} else {
			p.keep("-b:v", p.bv)
		}
	} else if p.bv != "" {
		if p.bv == "0" {
			p.recognize("%s", i18n.T("-b:v 0 (нужен для режима качества NVENC; форма добавляет его сама)"))
		} else {
			p.keep("-b:v", p.bv)
		}
	}
	if qv != "" {
		if f, err := strconv.ParseFloat(qv, 64); err == nil {
			v.Quality = int(math.Round(f))
			if f != math.Round(f) {
				p.warn(i18n.T("Качество %s округлено до %d."), qv, v.Quality)
			}
			v.RateMode = preset.RateQuality
			p.recognize(i18n.T("Качество: %d"), v.Quality)
		} else {
			p.keep("-crf", qv)
		}
	}
	// the rest of crf/cq/gq values that were not used
	for flag, val := range map[string]string{"-crf": p.crf, "-cq": p.cq, "-global_quality": p.gq} {
		if val != "" && val != qv {
			p.keep(flag, val)
		}
	}
	if p.maxrate != "" {
		p.keep("-maxrate", p.maxrate)
	}
	for _, flag := range []string{"-qp_i", "-qp_p", "-qp_b"} {
		if val, ok := p.qp[flag]; ok {
			p.keep(flag, val)
		}
	}
	if p.rc != "" {
		if oneOf(p.rc, "vbr", "cqp", "vbr_peak") && v.Codec != preset.CodecCopy && family != preset.EncCPU {
			p.recognize(i18n.T("-rc %s (форма задаёт сама)"), p.rc)
		} else {
			p.keep("-rc", p.rc)
		}
	}

	// speed
	raw := p.preset
	if p.qualityFlag != "" && family == preset.EncAMF {
		raw = p.qualityFlag
	} else if p.qualityFlag != "" {
		p.keep("-quality", p.qualityFlag)
	}
	if raw != "" {
		v.Speed = raw
		for _, lvl := range preset.SpeedLevels {
			if cmdbuild.SpeedValue(v.Codec, family, lvl) == raw {
				v.Speed = lvl
				break
			}
		}
		p.recognize(i18n.T("Скорость кодирования: %s"), raw)
	}

	// fps from -r
	if p.fpsOpt != "" {
		if f := atof(p.fpsOpt); f > 0 && v.FPS == 0 {
			v.FPS = f
			p.recognize(i18n.T("Частота кадров: %s fps (-r)"), p.fpsOpt)
		} else {
			p.keep("-r", p.fpsOpt)
		}
	}

	// container
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(p.outName)), ".")

	// hvc1 tag
	if p.tagHvc1 {
		if v.Codec == preset.CodecH265 && (ext == "" || ext == "mp4" || ext == "mov" || ext == "m4v") {
			p.recognize("%s", i18n.T("-tag:v hvc1 (форма добавляет для H.265 в mp4/mov сама)"))
		} else {
			p.keep("-tag:v", "hvc1")
		}
	}

	// audio
	if !p.haveACodec {
		p.warn("%s", i18n.T("Аудиокодек (-c:a) не указан: выбрано «копировать». ffmpeg по умолчанию перекодировал бы звук."))
	}
	if a.Mode == preset.AudioAAC || a.Mode == preset.AudioOpus {
		a.Bitrate = p.audioBitrate
		name := map[string]string{"aac": "AAC", "opus": "Opus"}[a.Mode]
		if p.audioBitrate != "" {
			p.recognize(i18n.T("Звук: %s, %s"), name, p.audioBitrate)
		} else {
			p.recognize(i18n.T("Звук: %s (битрейт по умолчанию)"), name)
		}
	} else if p.audioBitrate != "" {
		p.keep("-b:a", p.audioBitrate)
	}

	out := preset.Preset{
		Ext:   ext,
		VF:    strings.Join(p.vfLeft, ","),
		AF:    strings.Join(p.afLeft, ","),
		Args:  p.extra,
		Video: &v, Audio: &a,
	}
	p.res.Preset = out
	if out.VF != "" {
		p.res.Extra = append(p.res.Extra, "-vf "+out.VF)
	}
	if out.AF != "" {
		p.res.Extra = append(p.res.Extra, "-af "+out.AF)
	}
	if len(p.extra) > 0 {
		p.res.Extra = append(p.res.Extra, preset.JoinArgs(p.extra))
	}
	return p.res
}

func oneOf(s string, set ...string) bool {
	for _, x := range set {
		if s == x {
			return true
		}
	}
	return false
}
