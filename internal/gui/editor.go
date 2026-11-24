package gui

import (
	"bytes"
	"fmt"
	"regexp"
	"soverter/internal/i18n"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"soverter/internal/cmdbuild"
	"soverter/internal/encoders"
	"soverter/internal/ffimport"
	"soverter/internal/preset"
	"soverter/internal/runner"
)

// EditorHooks connects the editor to the OS-specific parts.
type EditorHooks struct {
	Install         func(*preset.Config) error                           // register menu entries
	Uninstall       func() error                                         // remove menu entries
	Untrust         func() error                                         // remove the trusted signing certificate (UAC)
	Installed       func() bool                                          // is the menu registered
	ModernAvailable func() bool                                          // does this build carry the Windows 11 menu package
	Test            func(p preset.Preset, file string, c *preset.Config) // run unsaved preset on a file
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug makes an id candidate from a name.
func Slug(name string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		s = "preset"
	}
	return s
}

// Editor is the preset editor view. All form state is derived from the selected preset every frame; the only
// copies are the text fields, which are reloaded when the selection changes.
type Editor struct {
	win   *Win
	path  string
	cfg   *preset.Config // working copy
	saved []byte         // encoded form of what is on disk
	hooks EditorHooks
	sel   int
	dirty bool

	// preset drag-and-drop reordering
	dragIdx  int // index of the row being dragged, -1 if none
	dragDY   float32
	rowPitch int

	sizeCustom bool // "custom resolution" was chosen explicitly (values alone cannot tell)

	enc       *encoders.Manager
	mu        sync.Mutex // guards ffVersion / probeErr / probing
	ffVersion string
	probeErr  error
	probing   bool

	issues      []cmdbuild.Issue
	previewText string
	argsErr     error
	dlg         *Dialog
	toast       string
	toastUntil  time.Time

	// widgets
	list          widget.List
	scroll        widget.List
	rows          []presetRow
	bAdd, bDup    IconBtn
	bDel, bUp     IconBtn
	bDown         IconBtn
	bSave, bTest  Btn
	bRevert       Btn
	bMenu, bSet   Btn
	bImport       Btn
	bConvert      Btn
	bCopy         Btn
	fName, fID    *Field
	fSuffix       *Field
	fRawVF        *Field
	fRawAF        *Field
	fRawArgs      *Field
	fBitrate      *Field
	fSizeMB       *Field
	fW, fH        *Field
	fFPS, fVol    *Field
	fVF, fAF      *Field
	fArgs         *Field
	sCodec, sEnc  Seg
	sRate, sSpeed Seg
	sSize, sScale Seg
	sFPS, sAMode  Seg
	sABit, sExt   Seg
	slider        Slider

	loaded int // selection the text fields currently show (-2 = force reload)
	// dialog state
	imp   importState
	setSt settingsState
}

// newEditor builds the editor state without a window.
func newEditor(path string, cfg *preset.Config, hooks EditorHooks) *Editor {
	e := &Editor{path: path, cfg: cfg, hooks: hooks, enc: &encoders.Manager{}, loaded: -2, dragIdx: -1}
	e.list.Axis = 1
	e.scroll.Axis = 1
	for _, f := range []**Field{&e.fName, &e.fID, &e.fSuffix, &e.fRawVF, &e.fRawAF, &e.fRawArgs,
		&e.fBitrate, &e.fSizeMB, &e.fW, &e.fH, &e.fFPS, &e.fVol, &e.fVF, &e.fAF, &e.fArgs} {
		*f = NewField()
	}
	for _, f := range []*Field{e.fRawVF, e.fRawAF, e.fRawArgs, e.fVF, e.fAF, e.fArgs} {
		f.Mono = true
	}
	e.snapshot()
	if len(cfg.Presets) > 0 {
		e.sel = 0
	} else {
		e.sel = -1
	}
	return e
}

// NewEditor builds the editor and opens its window.
func NewEditor(path string, cfg *preset.Config, hooks EditorHooks) *Editor {
	e := newEditor(path, cfg, hooks)
	e.win = Open("Soverter", 1180, 780, e)
	e.win.OnScale = func(p int) { e.persistUI(func(s *preset.Settings) { s.UIScale = p }) }
	e.startProbe(false)
	if hooks.Installed != nil && !hooks.Installed() {
		e.confirm(i18n.T("Меню не установлено"), i18n.T("Пункты в контекстном меню Explorer ещё не созданы. Установить сейчас?"), i18n.T("Установить"), false, e.installMenu)
	}
	return e
}

// ---- state helpers -------------------------------------------------------------------------------------------

func (e *Editor) snapshot() { e.saved, _ = preset.Encode(e.cfg); e.dirty = false }

func (e *Editor) recomputeDirty() {
	cur, _ := preset.Encode(e.cfg)
	e.dirty = !bytes.Equal(cur, e.saved)
	t := "Soverter"
	if e.dirty {
		t += " *"
	}
	e.win.SetTitle(t)
}

func (e *Editor) cur() *preset.Preset {
	if e.sel < 0 || e.sel >= len(e.cfg.Presets) {
		return nil
	}
	return &e.cfg.Presets[e.sel]
}

// CanClose asks before discarding unsaved changes.
func (e *Editor) CanClose() bool {
	if !e.dirty {
		return true
	}
	e.confirm(i18n.T("Несохранённые изменения"), i18n.T("Закрыть редактор и потерять изменения?"), i18n.T("Закрыть без сохранения"), true, func() { e.win.Close() })
	return false
}

// touch must be called after every change of the selected preset or settings.
func (e *Editor) touch() {
	e.argsErr = nil
	if c := e.cur(); c != nil {
		src := e.fRawArgs
		if c.Structured() {
			src = e.fArgs
		}
		if _, err := preset.SplitArgs(src.Text()); err != nil {
			e.argsErr = err
		}
	}
	e.recomputeDirty()
	e.refreshPreview()
}

func (e *Editor) selectPreset(i int) {
	e.sel = i
	e.sizeCustom = false
	e.loaded = -2
}

// loadFields copies the selected preset into the text fields.
func (e *Editor) loadFields() {
	e.loaded = e.sel
	e.sizeCustom = false
	c := e.cur()
	if c == nil {
		return
	}
	v, a := c.Normalized()
	e.fName.Set(c.Name)
	e.fID.Set(c.ID)
	e.fSuffix.Set(c.Suffix)
	e.fRawVF.Set(c.VF)
	e.fRawAF.Set(c.AF)
	e.fRawArgs.Set(preset.JoinArgs(c.Args))
	e.fVF.Set(c.VF)
	e.fAF.Set(c.AF)
	e.fArgs.Set(preset.JoinArgs(c.Args))
	e.fBitrate.Set(numOrEmpty(v.VideoBitrate))
	e.fSizeMB.Set(numOrEmpty(v.TargetSizeMB))
	e.fW.Set(intOrEmpty(v.Width))
	e.fH.Set(intOrEmpty(v.Height))
	e.fFPS.Set(numOrEmpty(v.FPS))
	e.fVol.Set(numOrEmpty(a.VolumeDB))
	e.touch()
}

// pollFields applies text edits made by the user to the selected preset; reports whether anything changed.
func (e *Editor) pollFields() bool {
	c := e.cur()
	if c == nil {
		return false
	}
	changed := false
	if e.fName.Changed() {
		s := e.fName.Text()
		if c.ID == Slug(c.Name) || c.ID == "" {
			c.ID = Slug(s)
			e.fID.Set(c.ID)
		}
		c.Name = s
		changed = true
	}
	if e.fID.Changed() {
		c.ID = e.fID.Text()
		changed = true
	}
	if e.fSuffix.Changed() {
		c.Suffix = e.fSuffix.Text()
		changed = true
	}
	if c.Structured() {
		if e.fBitrate.Changed() {
			e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.VideoBitrate = parseNum(e.fBitrate.Text()) })
			changed = true
		}
		if e.fSizeMB.Changed() {
			e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.TargetSizeMB = parseNum(e.fSizeMB.Text()) })
			changed = true
		}
		if e.fW.Changed() || e.fH.Changed() {
			e.editVideo(func(v *preset.Video, _ *preset.Audio) {
				v.Width, v.Height = int(parseNum(e.fW.Text())), int(parseNum(e.fH.Text()))
			})
			changed = true
		}
		if e.fFPS.Changed() {
			e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.FPS = parseNum(e.fFPS.Text()) })
			changed = true
		}
		if e.fVol.Changed() {
			e.editVideo(func(_ *preset.Video, a *preset.Audio) { a.VolumeDB = parseNum(e.fVol.Text()) })
			changed = true
		}
		if e.fVF.Changed() {
			c.VF = strings.TrimSpace(e.fVF.Text())
			changed = true
		}
		if e.fAF.Changed() {
			c.AF = strings.TrimSpace(e.fAF.Text())
			changed = true
		}
		if e.fArgs.Changed() {
			if a, err := preset.SplitArgs(e.fArgs.Text()); err == nil {
				c.Args = a
			}
			changed = true
		}
	} else {
		if e.fRawVF.Changed() {
			c.VF = e.fRawVF.Text()
			changed = true
		}
		if e.fRawAF.Changed() {
			c.AF = e.fRawAF.Text()
			changed = true
		}
		if e.fRawArgs.Changed() {
			if a, err := preset.SplitArgs(e.fRawArgs.Text()); err == nil {
				c.Args = a
			}
			changed = true
		}
	}
	if changed {
		e.touch()
	}
	return changed
}

// editVideo changes the structured settings of the selected preset and applies the cross-field rules:
// switching to "no re-encoding" drops scale/fps/rate control, and a volume change forces audio re-encoding.
func (e *Editor) editVideo(f func(v *preset.Video, a *preset.Audio)) {
	p := e.cur()
	if p == nil || !p.Structured() {
		return
	}
	v, a := p.Normalized()
	prev := v.Codec
	f(&v, &a)
	if v.Codec == preset.CodecCopy && prev != preset.CodecCopy {
		v.ScaleMode, v.Width, v.Height, v.FPS, v.RateMode = preset.ScaleNone, 0, 0, 0, preset.RateQuality
		e.fW.Set("")
		e.fH.Set("")
		e.fFPS.Set("")
	}
	if a.Mode == preset.AudioCopy && a.VolumeDB != 0 {
		a.Mode = preset.AudioAAC
		if a.Bitrate == "" {
			a.Bitrate = "192k"
		}
	}
	if (a.Mode == preset.AudioAAC || a.Mode == preset.AudioOpus) && a.Bitrate == "" {
		a.Bitrate = "192k"
	}
	p.Video, p.Audio = &v, &a
	e.touch()
}

func (e *Editor) setExt(ext string) {
	if c := e.cur(); c != nil {
		c.Ext = ext
		e.touch()
	}
}

// ---- encoders / ffmpeg ---------------------------------------------------------------------------------------

func (e *Editor) encReport() (encoders.Report, bool) {
	r, ready, _ := e.enc.Report()
	return r, ready
}

// startProbe checks the encoders in the background (cached; force = «Перепроверить»).
func (e *Editor) startProbe(force bool) {
	e.mu.Lock()
	e.probing = true
	e.probeErr = nil
	e.mu.Unlock()
	path := e.cfg.Settings.FFmpegPath
	go func() {
		t, err := runner.FindTools(path)
		if err != nil {
			e.mu.Lock()
			e.ffVersion, e.probing = "", false
			e.probeErr = fmt.Errorf("%s", i18n.T("ffmpeg не найден"))
			e.mu.Unlock()
			e.win.Invalidate()
			return
		}
		ver := t.Version()
		e.mu.Lock()
		e.ffVersion = ver
		e.mu.Unlock()
		e.win.Invalidate()
		e.enc.Refresh(t.FFmpeg, force, func(_ encoders.Report, err error) {
			e.mu.Lock()
			e.probing, e.probeErr = false, err
			e.mu.Unlock()
			e.win.Invalidate()
		})
	}()
}

// statusText renders the status bar from the ffmpeg version and the encoder report.
func (e *Editor) statusText() string {
	e.mu.Lock()
	ver, perr, probing := e.ffVersion, e.probeErr, e.probing
	e.mu.Unlock()
	if i := strings.Index(ver, " Copyright"); i > 0 {
		ver = ver[:i]
	}
	ver = strings.Replace(ver, "version ", "", 1)
	s := ver
	if s == "" {
		s = "ffmpeg"
	}
	switch rep, ready := e.encReport(); {
	case perr != nil:
		s += " · " + perr.Error()
	case probing || !ready:
		s += i18n.T(" · проверка энкодеров…")
	default:
		var hw []string
		for _, f := range familyNames()[1:] {
			var codecs []string
			for _, c := range codecOpts()[1:] {
				if rep.OK(cmdbuild.EncoderName(c.Val, f.Val)) {
					codecs = append(codecs, strings.Fields(c.Label)[0])
				}
			}
			if len(codecs) > 0 {
				hw = append(hw, fmt.Sprintf("%s (%s)", f.Label, strings.Join(codecs, ", ")))
			}
		}
		if len(hw) == 0 {
			s += i18n.T(" · аппаратных энкодеров нет")
		} else {
			s += i18n.T(" · аппаратные: ") + strings.Join(hw, ", ")
		}
	}
	if e.hooks.Installed != nil && e.hooks.Installed() {
		s += i18n.T(" · меню установлено")
	} else {
		s += i18n.T(" · меню не установлено")
	}
	return s
}

// encoderHint explains which encoders are unavailable for the codec on this machine.
func (e *Editor) encoderHint(codec, current string) string {
	rep, ready := e.encReport()
	if !ready {
		return i18n.T("Проверяю, какие энкодеры доступны на этой машине…")
	}
	var down []string
	for _, f := range familyNames() {
		enc := cmdbuild.EncoderName(codec, f.Val)
		if !rep.OK(enc) {
			down = append(down, f.Label+": "+rep.Why(enc))
		}
	}
	if len(down) == 0 {
		return hwHint()
	}
	return i18n.T("Недоступно на этой машине:\n• ") + strings.Join(down, "\n• ") + "\n" + hwHint()
}

// ---- preview / validation ------------------------------------------------------------------------------------

func (e *Editor) refreshPreview() {
	c := e.cur()
	if c == nil {
		return
	}
	var issues []cmdbuild.Issue
	if e.argsErr != nil {
		issues = append(issues, cmdbuild.Issue{Severity: cmdbuild.Error, Msg: i18n.T("Аргументы: ") + e.argsErr.Error()})
	}
	if c.Structured() {
		issues = append(issues, cmdbuild.Validate(*c)...)
		if rep, ready := e.encReport(); ready {
			issues = append(issues, cmdbuild.CheckAvailability(*c, rep)...)
		}
	}
	e.issues = issues

	ext := strings.Replace(c.Ext, "same", "mp4", 1)
	out := "output." + c.Suffix + "." + ext
	var lines []string
	if cmdbuild.NeedsTwoPass(*c) {
		for pass := 1; pass <= 2; pass++ {
			if a, err := cmdbuild.Args(*c, "input.mp4", out, cmdbuild.Opts{SrcExt: ".mp4", Preview: true, Pass: pass, PassLog: "pass"}); err == nil {
				lines = append(lines, fmt.Sprintf(i18n.T("# проход %d из 2"), pass), "ffmpeg "+preset.JoinArgs(a))
			}
		}
	} else if a, err := cmdbuild.Args(*c, "input.mp4", out, cmdbuild.Opts{SrcExt: ".mp4", Preview: true}); err == nil {
		lines = append(lines, "ffmpeg "+preset.JoinArgs(a))
	}
	if len(lines) == 0 {
		e.previewText = ""
		return
	}
	e.previewText = strings.Join(lines, "\n")
}

// validate checks every preset before saving; errors block the save, warnings do not.
func (e *Editor) validate() error {
	if err := preset.ValidateAll(e.cfg.Presets); err != nil {
		return err
	}
	for _, p := range e.cfg.Presets {
		for _, is := range cmdbuild.Validate(p) {
			if is.Severity == cmdbuild.Error {
				return fmt.Errorf(i18n.T("пресет «%s»: %s"), p.Name, is.Msg)
			}
		}
	}
	if e.argsErr != nil {
		return fmt.Errorf(i18n.T("пресет «%s»: аргументы: %w"), e.cur().Name, e.argsErr)
	}
	return nil
}

// ---- actions -------------------------------------------------------------------------------------------------

func (e *Editor) freeID(base string) string {
	id := base
	for n := 2; e.hasID(id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func (e *Editor) hasID(id string) bool {
	for _, p := range e.cfg.Presets {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (e *Editor) addPreset() {
	e.cfg.Presets = append(e.cfg.Presets, newStructured(e.freeID("new")))
	e.selectPreset(len(e.cfg.Presets) - 1)
	e.recomputeDirty()
}

func (e *Editor) dupPreset() {
	c := e.cur()
	if c == nil {
		return
	}
	n := clonePreset(*c)
	n.ID = e.freeID(c.ID + "-copy")
	n.Name += i18n.T(" (копия)")
	e.cfg.Presets = append(e.cfg.Presets, n)
	e.selectPreset(len(e.cfg.Presets) - 1)
	e.recomputeDirty()
}

func (e *Editor) deletePreset() {
	c := e.cur()
	if c == nil {
		return
	}
	e.confirm(i18n.T("Удалить пресет"), i18n.T("Удалить «")+c.Name+i18n.T("»? Чтобы просто убрать его из меню, выключите переключатель в списке."), i18n.T("Удалить"), true, func() {
		i := e.sel
		e.cfg.Presets = append(e.cfg.Presets[:i], e.cfg.Presets[i+1:]...)
		if i >= len(e.cfg.Presets) {
			i = len(e.cfg.Presets) - 1
		}
		e.selectPreset(i)
		e.recomputeDirty()
	})
}

func (e *Editor) move(d int) {
	i, j := e.sel, e.sel+d
	if i < 0 || j < 0 || j >= len(e.cfg.Presets) {
		return
	}
	ps := e.cfg.Presets
	ps[i], ps[j] = ps[j], ps[i]
	e.sel = j
	e.recomputeDirty()
}

func (e *Editor) save() {
	if err := e.validate(); err != nil {
		e.info(i18n.T("Не удалось сохранить"), err.Error(), true)
		return
	}
	if err := preset.Save(e.path, e.cfg); err != nil {
		e.info(i18n.T("Не удалось сохранить"), err.Error(), true)
		return
	}
	e.snapshot()
	e.recomputeDirty()
	if err := e.hooks.Install(e.cfg); err != nil {
		e.info(i18n.T("Пресеты сохранены, но меню не обновлено"), err.Error(), true)
		return
	}
	e.setToast(i18n.T("Сохранено, меню обновлено"))
}

func (e *Editor) installMenu() {
	c, err := preset.Load(e.path)
	if err != nil {
		e.info(i18n.T("Меню"), err.Error(), true)
		return
	}
	if err := e.hooks.Install(c); err != nil {
		e.info(i18n.T("Меню"), err.Error(), true)
		return
	}
	e.setToast(i18n.T("Меню установлено по сохранённым пресетам"))
}

func (e *Editor) revert() {
	e.confirm(i18n.T("Отменить правки"), i18n.T("Отбросить все несохранённые изменения и вернуть пресеты из файла presets.toml?"), i18n.T("Отбросить"), true, func() {
		c, err := preset.Load(e.path)
		if err != nil {
			e.info(i18n.T("Не удалось загрузить"), err.Error(), true)
			return
		}
		e.cfg = c
		e.snapshot()
		e.recomputeDirty()
		if len(c.Presets) > 0 {
			e.selectPreset(0)
		} else {
			e.selectPreset(-1)
		}
	})
}

// test runs the current (possibly unsaved) preset on a chosen file.
func (e *Editor) test() {
	c := e.cur()
	if c == nil {
		return
	}
	if err := c.Validate(); err != nil {
		e.info(i18n.T("Тест невозможен"), err.Error(), true)
		return
	}
	for _, is := range cmdbuild.Validate(*c) {
		if is.Severity == cmdbuild.Error {
			e.info(i18n.T("Тест невозможен"), is.Msg, true)
			return
		}
	}
	p := clonePreset(*c)
	cfg := e.cfg
	go func() {
		if path, ok := PickVideoFile(i18n.T("Выберите видео для теста")); ok && e.hooks.Test != nil {
			e.hooks.Test(p, path, cfg)
		}
	}()
}

// ---- dialogs -------------------------------------------------------------------------------------------------

func (e *Editor) closeDlg() {
	e.dlg = nil
	e.resetHover()
}

// resetHover forgets the "pointer is over me" state of the buttons under a dialog. While a dialog is open the
// layer below is laid out disabled and never sees the pointer leave, so whichever button the pointer was on when
// the dialog opened (usually the one that opened it) would stay highlighted after the dialog closes.
func (e *Editor) resetHover() {
	e.bAdd, e.bDup, e.bDel, e.bUp, e.bDown = IconBtn{}, IconBtn{}, IconBtn{}, IconBtn{}, IconBtn{}
	e.bSave, e.bTest, e.bRevert = Btn{}, Btn{}, Btn{}
	e.bMenu, e.bSet, e.bImport = Btn{}, Btn{}, Btn{}
	e.bConvert, e.bCopy = Btn{}, Btn{}
	for i := range e.rows {
		e.rows[i].click = widget.Clickable{}
	}
}

func (e *Editor) confirm(title, msg, ok string, danger bool, do func()) {
	kind := Primary
	if danger {
		kind = Secondary
	}
	e.dlg = &Dialog{
		Title: title, Close: e.closeDlg,
		Body: func(gtx C, th *Th) D { return th.Paragraph(gtx, 14, th.P.TextDim, msg) },
		Actions: []*DAction{
			{Label: i18n.T("Отмена"), Kind: Ghost, Do: e.closeDlg},
			{Label: ok, Kind: kind, Do: func() { e.closeDlg(); do() }},
		},
	}
	e.win.Invalidate()
}

func (e *Editor) info(title, msg string, isErr bool) {
	e.dlg = &Dialog{
		Title: title, Close: e.closeDlg,
		Body: func(gtx C, th *Th) D {
			if isErr {
				return Banner(gtx, th, "error", msg)
			}
			return th.Paragraph(gtx, 14, th.P.TextDim, msg)
		},
		Actions: []*DAction{{Label: i18n.T("Понятно"), Kind: Primary, Do: e.closeDlg}},
	}
	e.win.Invalidate()
}

// ---- import from an ffmpeg command ---------------------------------------------------------------------------

type importState struct {
	field  *Field
	report string
	res    *ffimport.Result
	last   string
}

func formatImport(r ffimport.Result) string {
	var b strings.Builder
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(title + "\n")
		for _, it := range items {
			b.WriteString("  • " + it + "\n")
		}
		b.WriteString("\n")
	}
	section(i18n.T("Распознано (попадёт в форму):"), r.Recognized)
	section(i18n.T("Останется в дополнительных аргументах (сырое):"), r.Extra)
	section(i18n.T("Предупреждения:"), r.Warnings)
	section(i18n.T("Не используется (ffmpeg, входной и выходной файлы, служебные флаги):"), r.Ignored)
	return strings.TrimSpace(b.String())
}

func importPrompt() string {
	return i18n.T("Вставьте команду ffmpeg: ниже появится, что распознано, а что останется сырым.")
}

func (e *Editor) importDialog(initial string) {
	if e.cur() == nil {
		return
	}
	st := &e.imp
	st.field = NewArea(5)
	st.field.Mono = true
	st.field.Set(initial)
	st.report, st.res, st.last = importPrompt(), nil, "\x00"
	e.dlg = &Dialog{
		Title: i18n.T("Импорт из команды ffmpeg"), Width: 720, Close: e.closeDlg,
		Body: func(gtx C, th *Th) D {
			if t := st.field.Text(); t != st.last {
				st.last = t
				st.res = nil
				if strings.TrimSpace(t) == "" {
					st.report = importPrompt()
				} else if r, err := ffimport.Parse(t); err != nil {
					st.report = "✖ " + err.Error()
				} else {
					st.res, st.report = &r, formatImport(r)
				}
			}
			return Vert(gtx, 12,
				func(gtx C) D {
					return st.field.Layout(gtx, th, `ffmpeg -i input.mp4 -vf "scale=1920:1080" -c:v libx264 -crf 18 -c:a copy output.mp4`)
				},
				func(gtx C) D { return th.Paragraph(gtx, 13, th.P.TextDim, st.report) })
		},
		Actions: []*DAction{
			{Label: i18n.T("Отмена"), Kind: Ghost, Do: e.closeDlg},
			{Label: i18n.T("Применить к пресету"), Kind: Primary, Do: func() {
				if st.res == nil {
					return
				}
				e.applyImport(*st.res)
				e.closeDlg()
			}},
		},
	}
}

// applyImport replaces the encoding settings of the selected preset; name, id and suffix stay.
func (e *Editor) applyImport(r ffimport.Result) {
	c := e.cur()
	if c == nil {
		return
	}
	c.Video, c.Audio = r.Preset.Video, r.Preset.Audio
	c.VF, c.AF, c.Args = r.Preset.VF, r.Preset.AF, r.Preset.Args
	if r.Preset.Ext != "" {
		c.Ext = r.Preset.Ext
	}
	e.loaded = -2
}

// convertLegacy opens the import dialog prefilled with the raw preset's command.
func (e *Editor) convertLegacy() {
	c := e.cur()
	if c == nil {
		return
	}
	a, err := cmdbuild.Args(*c, "input.mp4", "output.mp4", cmdbuild.Opts{})
	if err != nil {
		e.info(i18n.T("Импорт"), err.Error(), true)
		return
	}
	e.importDialog("ffmpeg " + preset.JoinArgs(a))
}

// ---- settings and Windows menu dialogs -----------------------------------------------------------------------

type settingsState struct {
	integration, mode string
	autoClose         Switch
	ffmpeg            *Field
	title             *Field
	sInt, sMode       Seg
	sLang, sScale     Seg
	sTheme            Seg
	bRecheck          Btn
}

func (e *Editor) settingsDialog() {
	st := &e.setSt
	st.integration, st.mode = e.cfg.Settings.Integration, e.cfg.Settings.MenuMode
	if st.integration != "modern" {
		st.integration = "classic"
	}
	if st.mode != "single" {
		st.mode = "submenu"
	}
	st.autoClose.Value = e.cfg.Settings.AutoClose
	st.ffmpeg = NewField()
	st.ffmpeg.Set(e.cfg.Settings.FFmpegPath)
	st.title = NewField()
	st.title.Set(e.cfg.Settings.MenuTitle)
	e.dlg = &Dialog{
		Title: i18n.T("Параметры"), Width: 560, Close: e.closeDlg,
		Body: func(gtx C, th *Th) D {
			return Vert(gtx, 16,
				Labeled(th, i18n.T("Язык"), func(gtx C) D {
					v, d := st.sLang.Layout(gtx, th, []Opt{{Val: "en", Label: "English"}, {Val: "ru", Label: "Русский"}}, i18n.Lang())
					if v != "" && v != i18n.Lang() {
						e.setLanguage(v)
					}
					return d
				}),
				Labeled(th, i18n.T("Тема"), func(gtx C) D {
					v, d := st.sTheme.Layout(gtx, th, []Opt{{Val: ThemeSystem, Label: i18n.T("Как в системе")}, {Val: ThemeDark, Label: i18n.T("Тёмная")}, {Val: ThemeLight, Label: i18n.T("Светлая")}}, ThemeMode())
					if v != "" && v != ThemeMode() {
						e.setTheme(v)
					}
					return d
				}),
				Labeled(th, i18n.T("Масштаб интерфейса"), func(gtx C) D {
					v, d := st.sScale.Layout(gtx, th, scaleOpts(), strconv.Itoa(Scale()))
					if n, err := strconv.Atoi(v); err == nil {
						e.setScale(n)
					}
					return d
				}),
				Hint(th, i18n.T("Ctrl + / Ctrl − / Ctrl 0 в любом окне меняют масштаб.")),
				Labeled(th, i18n.T("Интеграция с Explorer"), func(gtx C) D {
					opts := []Opt{{Val: "classic", Label: i18n.T("Классическое меню")}}
					if e.modernAvailable() {
						opts = append(opts, Opt{Val: "modern", Label: i18n.T("Меню Windows 11 (Beta)")})
					}
					v, d := st.sInt.Layout(gtx, th, opts, st.integration)
					if v != "" {
						st.integration = v
					}
					return d
				}),
				Hint(th, integrationHint(st.integration, e.modernAvailable())),
				Labeled(th, i18n.T("Название пункта в меню Explorer"), func(gtx C) D { return st.title.Layout(gtx, th, preset.DefaultMenuTitle()) }),
				Labeled(th, i18n.T("Вид меню"), func(gtx C) D {
					v, d := st.sMode.Layout(gtx, th, []Opt{{Val: "submenu", Label: i18n.T("Подменю")}, {Val: "single", Label: i18n.T("Один пункт с окном выбора")}}, st.mode)
					if v != "" {
						st.mode = v
					}
					return d
				}),
				func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D { return st.autoClose.Layout(gtx, th) }),
						layout.Rigid(layout.Spacer{Width: 12}.Layout),
						layout.Rigid(func(gtx C) D {
							return th.Text(gtx, 14, 400, th.P.Text, i18n.T("Закрывать окно прогресса после успеха"))
						}),
					)
				},
				Labeled(th, i18n.T("Путь к ffmpeg"), func(gtx C) D { return st.ffmpeg.Layout(gtx, th, i18n.T("пусто = искать в PATH")) }),
				func(gtx C) D {
					if st.bRecheck.Clicked(gtx) {
						path := strings.TrimSpace(st.ffmpeg.Text())
						_ = e.persistUI(func(s *preset.Settings) { s.FFmpegPath = path })
						e.startProbe(true)
						e.setToast(i18n.T("Энкодеры перепроверяются…"))
					}
					return st.bRecheck.Layout(gtx, th, i18n.T("Перепроверить энкодеры"), Secondary)
				},
			)
		},
		Actions: []*DAction{
			{Label: i18n.T("Отмена"), Kind: Ghost, Do: e.closeDlg},
			{Label: i18n.T("Готово"), Kind: Primary, Do: func() {
				e.applySettings(st)
			}},
		},
	}
}

func (e *Editor) modernAvailable() bool {
	return e.hooks.ModernAvailable == nil || e.hooks.ModernAvailable()
}

// integrationHint explains the chosen menu kind. The Windows 11 menu is a package signed with a self-signed
// certificate, so installing it needs administrator rights once (to trust that certificate).
func integrationHint(kind string, modern bool) string {
	if kind == "modern" && modern {
		return i18n.T("Бета. Пункт появится сразу в новом меню Windows 11, без «Показать дополнительные параметры». Меню ставится как пакет приложения, подписанный самоподписанным сертификатом (платного сертификата у проекта пока нет). Чтобы Windows его приняла, один раз потребуются права администратора (UAC): сертификат добавляется в доверенные. Убрать его можно в окне «Меню Windows».")
	}
	return i18n.T("Работает без прав администратора. В Windows 11 пункт находится в «Показать дополнительные параметры», в Windows 10 в обычном меню.")
}

type menuState struct{ bInst, bUninst, bCert Btn }

func (e *Editor) menuDialog() {
	st := &menuState{}
	e.dlg = &Dialog{
		Title: i18n.T("Меню Windows"), Width: 560, Close: e.closeDlg,
		Body: func(gtx C, th *Th) D {
			if st.bInst.Clicked(gtx) {
				e.closeDlg()
				e.installMenu()
			}
			if st.bUninst.Clicked(gtx) {
				e.closeDlg()
				e.confirm(i18n.T("Удалить меню"), i18n.T("Удалить пункты контекстного меню Soverter?"), i18n.T("Удалить"), true, func() {
					if err := e.hooks.Uninstall(); err != nil {
						e.info(i18n.T("Меню"), err.Error(), true)
						return
					}
					e.setToast(i18n.T("Меню удалено"))
				})
			}
			if st.bCert.Clicked(gtx) {
				e.closeDlg()
				e.confirm(i18n.T("Сертификат"), i18n.T("Удалить сертификат Soverter из доверенных? Новое меню Windows 11 нельзя будет установить, пока он не будет добавлен снова."), i18n.T("Удалить"), true, func() {
					if e.hooks.Untrust == nil {
						return
					}
					if err := e.hooks.Untrust(); err != nil {
						e.info(i18n.T("Сертификат"), err.Error(), true)
					}
				})
			}
			rows := []layout.Widget{
				Hint(th, i18n.T("Меню обновляется само при каждом «Сохранить». Здесь можно переустановить или убрать его вручную.")),
				func(gtx C) D {
					return st.bInst.Layout(gtx, th, i18n.T("Установить по сохранённым пресетам"), Primary)
				},
				func(gtx C) D { return st.bUninst.Layout(gtx, th, i18n.T("Удалить меню"), Secondary) },
			}
			// Only builds with an embedded self-signed certificate have one to remove; a package signed by a
			// trusted authority needs no trust step at all.
			// It is also hidden unless the Windows 11 menu is the chosen integration: the certificate only matters there.
			if e.hooks.Untrust != nil && e.cfg.Settings.Integration == "modern" {
				rows = append(rows, func(gtx C) D {
					return st.bCert.Layout(gtx, th, i18n.T("Удалить сертификат из доверенных (UAC)"), Ghost)
				})
			}
			return Vert(gtx, 12, rows...)
		},
		Actions: []*DAction{{Label: i18n.T("Закрыть"), Kind: Ghost, Do: e.closeDlg}},
	}
}

func scaleOpts() []Opt {
	var o []Opt
	cur := Scale()
	found := false
	for _, s := range []int{100, 110, 125, 150, 175, 200} {
		if s == cur {
			found = true
		}
		o = append(o, Opt{Val: strconv.Itoa(s), Label: fmt.Sprintf("%d%%", s)})
	}
	if !found {
		o = append(o, Opt{Val: strconv.Itoa(cur), Label: fmt.Sprintf("%d%%", cur)})
	}
	return o
}

// persistUI saves the language/scale to presets.toml right away without touching unsaved preset edits.
func (e *Editor) persistUI(mut func(*preset.Settings)) error {
	mut(&e.cfg.Settings)
	if e.path == "" {
		return nil
	}
	disk, err := preset.Load(e.path)
	if err != nil {
		return err // never overwrite a config we cannot read
	}
	mut(&disk.Settings)
	if err := preset.Save(e.path, disk); err != nil {
		return err
	}
	e.saved, _ = preset.Encode(disk)
	e.recomputeDirty()
	return nil
}

func (e *Editor) setLanguage(lang string) {
	i18n.SetLang(lang)
	e.persistUI(func(s *preset.Settings) { s.Language = lang })
	InvalidateAll()
	e.settingsDialog() // rebuild the dialog with translated captions
}

func (e *Editor) setScale(p int) {
	if p == Scale() {
		return
	}
	SetScale(p)
	e.persistUI(func(s *preset.Settings) { s.UIScale = p })
}

// applySettings is the «Готово» button of the settings dialog: the settings are written to presets.toml right
// away (unsaved preset edits are left alone) and the menu is re-registered if its kind or layout changed.
func (e *Editor) applySettings(st *settingsState) {
	cur := e.cfg.Settings
	curInt, curMode := cur.Integration, cur.MenuMode
	if curInt != "modern" {
		curInt = "classic"
	}
	if curMode != "single" {
		curMode = "submenu"
	}
	title := strings.TrimSpace(st.title.Text())
	menuChanged := curInt != st.integration || curMode != st.mode || title != strings.TrimSpace(cur.MenuTitle)
	path := strings.TrimSpace(st.ffmpeg.Text())
	err := e.persistUI(func(s *preset.Settings) {
		s.Integration, s.MenuMode = st.integration, st.mode
		s.AutoClose = st.autoClose.Value
		s.MenuTitle = title
		s.FFmpegPath = path
	})
	e.closeDlg()
	if err != nil {
		e.info(i18n.T("Не удалось сохранить"), err.Error(), true)
		return
	}
	e.startProbe(false)
	if menuChanged {
		e.installMenu() // re-register the menu in the new kind/layout (may ask for UAC)
		return
	}
	e.setToast(i18n.T("Настройки сохранены"))
}

// setTheme switches the theme of all windows at once and stores it (like the language, not tied to «Сохранить»).
func (e *Editor) setTheme(mode string) {
	SetThemeMode(mode)
	e.persistUI(func(s *preset.Settings) { s.Theme = mode })
}
