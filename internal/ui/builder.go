package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"vidpreset/internal/cmdbuild"
	"vidpreset/internal/preset"
)

type opt struct{ val, label string }

func labels(os []opt) []string {
	r := make([]string, len(os))
	for i, o := range os {
		r[i] = o.label
	}
	return r
}

func labelOf(os []opt, val string) string {
	for _, o := range os {
		if o.val == val {
			return o.label
		}
	}
	return ""
}

func valOf(os []opt, label string) string {
	for _, o := range os {
		if o.label == label {
			return o.val
		}
	}
	return ""
}

var (
	codecOpts = []opt{
		{preset.CodecCopy, "Без перекодирования"},
		{preset.CodecH264, "H.264"},
		{preset.CodecH265, "H.265 (HEVC)"},
		{preset.CodecAV1, "AV1"},
	}
	familyNames = []opt{
		{preset.EncCPU, "CPU (программный)"},
		{preset.EncNVENC, "NVIDIA NVENC"},
		{preset.EncAMF, "AMD AMF"},
		{preset.EncQSV, "Intel QSV"},
	}
	rateOpts = []opt{
		{preset.RateQuality, "По качеству"},
		{preset.RateBitrate, "По битрейту"},
		{preset.RateSize, "По размеру файла"},
	}
	speedOpts = []opt{
		{"", "По умолчанию энкодера"},
		{"fastest", "Самая быстрая"},
		{"fast", "Быстрая"},
		{"medium", "Средняя"},
		{"slow", "Медленная (лучше сжатие)"},
		{"slowest", "Самая медленная"},
	}
	scaleModeOpts = []opt{
		{preset.ScaleKeep, "Сохранить пропорции"},
		{preset.ScaleStretch, "Растянуть"},
		{preset.ScaleFit, "Вписать с чёрными полями"},
	}
	audioOpts = []opt{
		{preset.AudioCopy, "Копировать как есть"},
		{preset.AudioAAC, "Перекодировать в AAC"},
		{preset.AudioOpus, "Перекодировать в Opus"},
		{preset.AudioRemove, "Убрать звук"},
	}
	extOptions = []string{"same", "mp4", "mkv", "webm", "mov"}
)

type sizePreset struct {
	label string
	w, h  int // 0,0 = none / custom
}

var sizePresets = []sizePreset{
	{"Не менять", 0, 0},
	{"720p (1280×720)", 1280, 720},
	{"1080p (1920×1080)", 1920, 1080},
	{"1440p (2560×1440)", 2560, 1440},
	{"2160p (3840×2160)", 3840, 2160},
	{"Своё…", -1, -1},
}

const (
	sizeNone   = "Не менять"
	sizeCustom = "Своё…"
	fpsNone    = "не менять"
	hwHint     = "Аппаратное кодирование быстрее, но при том же размере файла качество обычно немного ниже, чем у libx264/libx265."
)

// sizeLabelFor picks the size-list entry matching a preset's scale settings.
func sizeLabelFor(v preset.Video) string {
	if v.ScaleMode == preset.ScaleNone {
		return sizeNone
	}
	for _, s := range sizePresets {
		if s.w <= 0 {
			continue
		}
		if v.ScaleMode == preset.ScaleKeep && v.Height == s.h {
			return s.label
		}
		if v.ScaleMode != preset.ScaleKeep && v.Width == s.w && v.Height == s.h {
			return s.label
		}
	}
	return sizeCustom
}

// builder is the visual preset constructor (the "form" mode).
type builder struct {
	e *editor

	cards *fyne.Container // all form cards for a structured preset

	codec    *widget.Select
	encoder  *widget.RadioGroup
	encHint  *widget.Label
	rate     *widget.RadioGroup
	quality  *widget.Slider
	qualVal  *widget.Label
	qualBox  *fyne.Container
	bitrate  *widget.Entry
	bitBox   *fyne.Container
	sizeMB   *widget.Entry
	sizeBox  *fyne.Container
	speed    *widget.Select
	speedMap *widget.Label
	videoDet *fyne.Container // rows that only make sense when transcoding

	sizeSel   *widget.Select
	scaleMode *widget.RadioGroup
	wEntry    *widget.Entry
	hEntry    *widget.Entry
	dimBox    *fyne.Container
	scaleBox  *fyne.Container
	fps       *widget.SelectEntry

	aMode    *widget.Select
	aBitrate *widget.SelectEntry
	aBitBox  *fyne.Container
	volume   *widget.Entry
	volNote  *widget.Label

	vf, af, args *widget.Entry
}

func newBuilder(e *editor) *builder {
	b := &builder{e: e}
	changed := func(string) { b.apply() }

	// --- video ---
	b.codec = widget.NewSelect(labels(codecOpts), changed)
	b.encoder = widget.NewRadioGroup(nil, changed)
	b.encoder.Horizontal = true
	b.encHint = widget.NewLabel("")
	b.encHint.Wrapping = fyne.TextWrapWord
	b.rate = widget.NewRadioGroup(labels(rateOpts), changed)
	b.rate.Horizontal = true
	b.rate.Required = true

	b.quality = widget.NewSlider(0, 51)
	b.quality.Step = 1
	b.qualVal = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	b.quality.OnChanged = func(float64) { b.apply() }
	ends := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("← лучше качество", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		widget.NewLabelWithStyle("меньше размер →", fyne.TextAlignTrailing, fyne.TextStyle{Italic: true}),
		b.qualVal)
	b.qualBox = container.NewVBox(b.quality, ends)

	b.bitrate = widget.NewEntry()
	b.bitrate.SetPlaceHolder("например 5")
	b.bitrate.OnChanged = changed
	b.bitBox = container.NewBorder(nil, nil, nil, widget.NewLabel("Мбит/с"), b.bitrate)
	b.sizeMB = widget.NewEntry()
	b.sizeMB.SetPlaceHolder("например 50")
	b.sizeMB.OnChanged = changed
	sizeHint := widget.NewLabel("Битрейт считается при запуске по длительности файла и звуку (с запасом 5%). " +
		"CPU H.264/H.265 кодируют в два прохода — точнее; аппаратные энкодеры и AV1 — в один, попадание в размер не гарантируется.")
	sizeHint.Wrapping = fyne.TextWrapWord
	b.sizeBox = container.NewVBox(container.NewBorder(nil, nil, nil, widget.NewLabel("МБ"), b.sizeMB), sizeHint)

	b.speed = widget.NewSelect(labels(speedOpts), changed)
	b.speedMap = widget.NewLabel("")

	b.videoDet = container.NewVBox(
		widget.NewForm(widget.NewFormItem("Энкодер", container.NewVBox(b.encoder, b.encHint))),
		widget.NewForm(widget.NewFormItem("Режим", b.rate)),
		b.qualBox, b.bitBox, b.sizeBox,
		widget.NewForm(widget.NewFormItem("Скорость", container.NewVBox(b.speed, b.speedMap))),
	)
	videoCard := widget.NewCard("Видео", "", nil)
	videoCard.SetContent(container.NewVBox(widget.NewForm(widget.NewFormItem("Кодек", b.codec)), b.videoDet))

	// --- size + fps ---
	var sizeLabels []string
	for _, s := range sizePresets {
		sizeLabels = append(sizeLabels, s.label)
	}
	b.sizeSel = widget.NewSelect(sizeLabels, changed)
	b.scaleMode = widget.NewRadioGroup(labels(scaleModeOpts), changed)
	b.scaleMode.Required = true
	b.wEntry = widget.NewEntry()
	b.wEntry.SetPlaceHolder("ширина")
	b.wEntry.OnChanged = changed
	b.hEntry = widget.NewEntry()
	b.hEntry.SetPlaceHolder("высота")
	b.hEntry.OnChanged = changed
	b.dimBox = container.NewGridWithColumns(2, b.wEntry, b.hEntry)
	b.scaleBox = container.NewVBox(widget.NewForm(widget.NewFormItem("Режим", b.scaleMode)))
	b.fps = widget.NewSelectEntry([]string{fpsNone, "24", "25", "30", "50", "60"})
	b.fps.OnChanged = changed
	sizeCard := widget.NewCard("Размер кадра и fps", "Масштабирование выполняется на CPU",
		container.NewVBox(
			widget.NewForm(widget.NewFormItem("Размер", b.sizeSel)),
			b.dimBox, b.scaleBox,
			widget.NewForm(widget.NewFormItem("fps", b.fps)),
		))

	// --- audio ---
	b.aMode = widget.NewSelect(labels(audioOpts), changed)
	b.aBitrate = widget.NewSelectEntry([]string{"96k", "128k", "160k", "192k", "256k", "320k"})
	b.aBitrate.OnChanged = changed
	b.aBitBox = container.NewVBox(widget.NewForm(widget.NewFormItem("Битрейт", b.aBitrate)))
	b.volume = widget.NewEntry()
	b.volume.SetPlaceHolder("0 = не менять, например -6")
	b.volume.OnChanged = changed
	b.volNote = widget.NewLabel("")
	b.volNote.Wrapping = fyne.TextWrapWord
	audioCard := widget.NewCard("Звук", "", container.NewVBox(
		widget.NewForm(widget.NewFormItem("Звук", b.aMode)),
		b.aBitBox,
		widget.NewForm(widget.NewFormItem("Громкость, дБ", b.volume)),
		b.volNote,
	))

	// --- extras ---
	b.vf = widget.NewEntry()
	b.vf.SetPlaceHolder("дополнительные видеофильтры, например hqdn3d")
	b.vf.OnChanged = changed
	b.af = widget.NewEntry()
	b.af.SetPlaceHolder("дополнительные аудиофильтры")
	b.af.OnChanged = changed
	b.args = widget.NewEntry()
	b.args.SetPlaceHolder(`например -movflags +faststart (кавычки как в shell)`)
	b.args.OnChanged = changed
	extraCard := widget.NewCard("Дополнительно", "Всё, чего нет в форме. Идёт в конец команды, перед выходным файлом.",
		widget.NewForm(
			widget.NewFormItem("Видеофильтры (vf)", b.vf),
			widget.NewFormItem("Аудиофильтры (af)", b.af),
			widget.NewFormItem("Дополнительные аргументы", b.args),
		))

	b.cards = container.NewVBox(videoCard, sizeCard, audioCard, extraCard)
	return b
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func parseNum(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// encoderLabel is the radio-group label of a family for the given codec, with availability marks.
func (b *builder) encoderLabel(codec, family string) string {
	name := labelOf(familyNames, family)
	if codec == preset.CodecCopy {
		return name
	}
	enc := cmdbuild.EncoderName(codec, family)
	if rep, ready := b.e.encReport(); ready && !rep.OK(enc) {
		return name + " (недоступно)"
	}
	return name
}

// rebuildEncoderOptions lists the families that work for the codec on this machine. The preset's own
// choice is always kept in the list (marked unavailable) so opening a preset never silently changes it.
func (b *builder) rebuildEncoderOptions(codec, current string) (hint string) {
	rep, ready := b.e.encReport()
	var opts []string
	var down []string
	for _, f := range familyNames {
		enc := cmdbuild.EncoderName(codec, f.val)
		okHere := !ready || rep.OK(enc)
		switch {
		case okHere:
			opts = append(opts, labelOf(familyNames, f.val))
		case f.val == current:
			opts = append(opts, labelOf(familyNames, f.val)+" (недоступно)")
			down = append(down, f.label+": "+rep.Why(enc))
		default:
			down = append(down, f.label+": "+rep.Why(enc))
		}
	}
	b.encoder.Options = opts
	b.encoder.Refresh()
	switch {
	case !ready:
		hint = "Проверяю, какие энкодеры доступны на этой машине…\n"
	case len(down) > 0:
		hint = "Недоступно на этой машине:\n• " + strings.Join(down, "\n• ") + "\n"
	}
	return hint + hwHint
}

// load fills the widgets from the preset.
func (b *builder) load(p preset.Preset) {
	v, a := p.Normalized()
	b.codec.SetSelected(labelOf(codecOpts, v.Codec))
	hint := b.rebuildEncoderOptions(v.Codec, v.Encoder)
	b.encHint.SetText(hint)
	sel := b.encoderLabel(v.Codec, v.Encoder)
	if rep, ready := b.e.encReport(); ready && !rep.OK(cmdbuild.EncoderName(v.Codec, v.Encoder)) {
		sel = labelOf(familyNames, v.Encoder) + " (недоступно)"
	}
	b.encoder.SetSelected(sel)
	b.rate.SetSelected(labelOf(rateOpts, v.RateMode))
	b.quality.SetValue(float64(v.Quality))
	b.bitrate.SetText(numOrEmpty(v.VideoBitrate))
	b.sizeMB.SetText(numOrEmpty(v.TargetSizeMB))

	// speed: unified names, "" = default, or a raw value kept from an import
	sl := labels(speedOpts)
	label := labelOf(speedOpts, v.Speed)
	if label == "" {
		label = v.Speed + " (из импорта)"
		sl = append(sl, label)
	}
	b.speed.Options = sl
	b.speed.Refresh()
	b.speed.SetSelected(label)

	b.sizeSel.SetSelected(sizeLabelFor(v))
	b.scaleMode.SetSelected(labelOf(scaleModeOpts, orStr(v.ScaleMode, preset.ScaleKeep, v.ScaleMode == preset.ScaleNone)))
	b.wEntry.SetText(intOrEmpty(v.Width))
	b.hEntry.SetText(intOrEmpty(v.Height))
	if v.FPS > 0 {
		b.fps.SetText(fmtNum(v.FPS))
	} else {
		b.fps.SetText(fpsNone)
	}

	b.aMode.SetSelected(labelOf(audioOpts, a.Mode))
	b.aBitrate.SetText(a.Bitrate)
	b.volume.SetText(numOrEmpty(a.VolumeDB))

	b.vf.SetText(p.VF)
	b.af.SetText(p.AF)
	b.args.SetText(preset.JoinArgs(p.Args))
	b.layout(p)
}

func orStr(s, def string, useDef bool) string {
	if useDef {
		return def
	}
	return s
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

// apply reads the widgets into the selected preset and refreshes dependent UI.
func (b *builder) apply() {
	e := b.e
	if e.loading {
		return
	}
	p := e.cur()
	if p == nil || !p.Structured() {
		return
	}
	v, a := p.Normalized()
	prevCodec := v.Codec

	v.Codec = valOf(codecOpts, b.codec.Selected)
	if v.Codec == "" {
		v.Codec = preset.CodecH264
	}
	if f := valOf(familyNames, strings.TrimSuffix(b.encoder.Selected, " (недоступно)")); f != "" {
		v.Encoder = f
	}
	v.RateMode = valOf(rateOpts, b.rate.Selected)
	if v.RateMode == "" {
		v.RateMode = preset.RateQuality
	}
	v.Quality = int(b.quality.Value)
	v.VideoBitrate = parseNum(b.bitrate.Text)
	v.TargetSizeMB = parseNum(b.sizeMB.Text)
	if sp := valOf(speedOpts, b.speed.Selected); sp != "" || b.speed.Selected == speedOpts[0].label {
		v.Speed = sp
	} else {
		v.Speed = strings.TrimSuffix(b.speed.Selected, " (из импорта)")
	}

	// size
	switch b.sizeSel.Selected {
	case sizeNone, "":
		v.ScaleMode = preset.ScaleNone
	case sizeCustom:
		v.ScaleMode = valOf(scaleModeOpts, b.scaleMode.Selected)
		if v.ScaleMode == "" {
			v.ScaleMode = preset.ScaleKeep
		}
		v.Width, v.Height = int(parseNum(b.wEntry.Text)), int(parseNum(b.hEntry.Text))
	default:
		v.ScaleMode = valOf(scaleModeOpts, b.scaleMode.Selected)
		if v.ScaleMode == "" {
			v.ScaleMode = preset.ScaleKeep
		}
		for _, s := range sizePresets {
			if s.label == b.sizeSel.Selected {
				v.Width, v.Height = s.w, s.h
			}
		}
	}
	if t := strings.TrimSpace(b.fps.Text); t == "" || t == fpsNone {
		v.FPS = 0
	} else {
		v.FPS = parseNum(t)
	}

	// without re-encoding there is nothing to scale or rate-control: reset instead of raising errors
	if v.Codec == preset.CodecCopy && prevCodec != preset.CodecCopy {
		v.ScaleMode, v.Width, v.Height, v.FPS, v.RateMode = preset.ScaleNone, 0, 0, 0, preset.RateQuality
	}

	a.Mode = valOf(audioOpts, b.aMode.Selected)
	if a.Mode == "" {
		a.Mode = preset.AudioCopy
	}
	a.Bitrate = strings.TrimSpace(b.aBitrate.Text)
	a.VolumeDB = parseNum(b.volume.Text)
	// volume needs re-encoding: switch copy -> AAC and say so (see volNote)
	if a.Mode == preset.AudioCopy && a.VolumeDB != 0 {
		a.Mode = preset.AudioAAC
		if a.Bitrate == "" {
			a.Bitrate = "192k"
		}
	}

	p.Video, p.Audio = &v, &a
	p.VF, p.AF = strings.TrimSpace(b.vf.Text), strings.TrimSpace(b.af.Text)
	if args, err := preset.SplitArgs(b.args.Text); err == nil {
		p.Args = args
	}

	// widgets that the rules above changed
	e.loading = true
	if v.Codec != prevCodec || b.encoder.Selected == "" {
		hint := b.rebuildEncoderOptions(v.Codec, v.Encoder)
		b.encHint.SetText(hint)
		b.encoder.SetSelected(b.encoderLabel(v.Codec, v.Encoder))
	}
	if sizeLabelFor(v) != b.sizeSel.Selected && v.ScaleMode == preset.ScaleNone {
		b.sizeSel.SetSelected(sizeNone)
		b.wEntry.SetText("")
		b.hEntry.SetText("")
		b.fps.SetText(fpsNone)
	}
	if b.aMode.Selected != labelOf(audioOpts, a.Mode) {
		b.aMode.SetSelected(labelOf(audioOpts, a.Mode))
		b.aBitrate.SetText(a.Bitrate)
	}
	if v.Codec == preset.CodecCopy && b.rate.Selected != labelOf(rateOpts, preset.RateQuality) {
		b.rate.SetSelected(labelOf(rateOpts, preset.RateQuality))
	}
	e.loading = false

	b.layout(*p)
	e.refreshPreview()
	e.markDirty()
}

// layout shows or hides rows according to the preset's choices.
func (b *builder) layout(p preset.Preset) {
	v, a := p.Normalized()
	copyV := v.Codec == preset.CodecCopy
	setVisible(b.videoDet, !copyV)
	setVisible(b.qualBox, !copyV && v.RateMode == preset.RateQuality)
	setVisible(b.bitBox, !copyV && v.RateMode == preset.RateBitrate)
	setVisible(b.sizeBox, !copyV && v.RateMode == preset.RateSize)
	b.qualVal.SetText(fmt.Sprintf("%d", v.Quality))

	custom := b.sizeSel.Selected == sizeCustom
	scaling := b.sizeSel.Selected != sizeNone && b.sizeSel.Selected != ""
	setVisible(b.scaleBox, scaling)
	setVisible(b.dimBox, custom)
	if custom && v.ScaleMode == preset.ScaleKeep {
		b.wEntry.Disable() // only the height matters for "keep proportions"
	} else {
		b.wEntry.Enable()
	}

	setVisible(b.aBitBox, a.Mode == preset.AudioAAC || a.Mode == preset.AudioOpus)
	switch {
	case a.VolumeDB != 0 && a.Mode != preset.AudioRemove:
		b.volNote.SetText("Изменение громкости требует перекодирования звука, поэтому звук не копируется как есть.")
	case a.VolumeDB != 0:
		b.volNote.SetText("Звук убирается: громкость не применится.")
	default:
		b.volNote.SetText("")
	}
	if !copyV && v.Speed != "" {
		flag := "-preset"
		if v.Encoder == preset.EncAMF {
			flag = "-quality"
		}
		b.speedMap.SetText("→ " + flag + " " + cmdbuild.SpeedValue(v.Codec, v.Encoder, v.Speed))
	} else {
		b.speedMap.SetText("")
	}
	b.cards.Refresh()
}

func setVisible(o fyne.CanvasObject, show bool) {
	if show {
		o.Show()
	} else {
		o.Hide()
	}
}

var _ = theme.ContentAddIcon

// refreshEncoders re-reads the encoder availability report into the encoder list (after the background probe).
func (b *builder) refreshEncoders(p preset.Preset) {
	v, _ := p.Normalized()
	e := b.e
	e.loading = true
	b.encHint.SetText(b.rebuildEncoderOptions(v.Codec, v.Encoder))
	label := b.encoderLabel(v.Codec, v.Encoder)
	if rep, ready := e.encReport(); ready && v.Codec != preset.CodecCopy && !rep.OK(cmdbuild.EncoderName(v.Codec, v.Encoder)) {
		label = labelOf(familyNames, v.Encoder) + " (недоступно)"
	}
	b.encoder.SetSelected(label)
	e.loading = false
}
