package ui

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"vidpreset/internal/cmdbuild"
	"vidpreset/internal/encoders"
	"vidpreset/internal/ffimport"
	"vidpreset/internal/preset"
	"vidpreset/internal/runner"
)

// EditorHooks connects the editor to the OS-specific parts.
type EditorHooks struct {
	Install   func(*preset.Config) error                           // register menu entries
	Uninstall func() error                                         // remove menu entries
	Untrust   func() error                                         // remove the trusted signing certificate (UAC)
	Installed func() bool                                          // is the menu registered
	Test      func(p preset.Preset, file string, c *preset.Config) // run unsaved preset on a file
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

type editor struct {
	app     fyne.App
	win     fyne.Window
	path    string
	cfg     *preset.Config // working copy
	saved   []byte         // encoded form of what is on disk
	hooks   EditorHooks
	sel     int
	loading bool

	enc       *encoders.Manager
	ffVersion string

	b *builder

	list                      *widget.List
	name, id, suffix          *widget.Entry
	rawVF, rawAF, rawArgs     *widget.Entry
	ext                       *widget.SelectEntry
	preview, status, dirtyLbl *widget.Label
	previewText               string
	issuesBox                 *fyne.Container
	saveBtn, revertBtn        *widget.Button
	rawCard                   fyne.CanvasObject
	form                      fyne.CanvasObject
	empty                     *widget.Label
	right                     *fyne.Container
}

// Editor opens the preset editor window.
func Editor(a fyne.App, path string, cfg *preset.Config, hooks EditorHooks) {
	e := &editor{app: a, path: path, cfg: cfg, hooks: hooks, sel: -1, enc: &encoders.Manager{}}
	e.snapshot()
	e.win = a.NewWindow("vidpreset — редактор пресетов")
	e.build()
	e.win.Resize(fyne.NewSize(1040, 720))
	e.win.SetCloseIntercept(e.onClose)
	e.win.Show()
	if len(e.cfg.Presets) > 0 {
		e.list.Select(0)
	} else {
		e.showForm(false)
	}
	e.markDirty()
	e.startProbe(false)
	if hooks.Installed != nil && !hooks.Installed() {
		dialog.ShowConfirm("Меню не установлено",
			"Пункты в контекстном меню Explorer ещё не созданы. Установить сейчас?",
			func(ok bool) {
				if ok {
					e.installMenu()
				}
			}, e.win)
	}
}

func (e *editor) snapshot() { e.saved, _ = preset.Encode(e.cfg) }

func (e *editor) dirty() bool {
	cur, _ := preset.Encode(e.cfg)
	return !bytes.Equal(cur, e.saved)
}

func (e *editor) markDirty() {
	d := e.dirty()
	if d {
		e.saveBtn.Enable()
		e.revertBtn.Enable()
		e.dirtyLbl.SetText("● есть несохранённые изменения")
		e.win.SetTitle("vidpreset — редактор пресетов *")
	} else {
		e.saveBtn.Disable()
		e.revertBtn.Disable()
		e.dirtyLbl.SetText("")
		e.win.SetTitle("vidpreset — редактор пресетов")
	}
}

func (e *editor) onClose() {
	if !e.dirty() {
		e.win.Close()
		e.app.Quit()
		return
	}
	dialog.ShowConfirm("Несохранённые изменения", "Закрыть редактор и потерять изменения?", func(ok bool) {
		if ok {
			e.win.Close()
			e.app.Quit()
		}
	}, e.win)
}

func (e *editor) cur() *preset.Preset {
	if e.sel < 0 || e.sel >= len(e.cfg.Presets) {
		return nil
	}
	return &e.cfg.Presets[e.sel]
}

func (e *editor) showForm(show bool) {
	if show {
		e.right.Objects = []fyne.CanvasObject{e.form}
	} else {
		e.right.Objects = []fyne.CanvasObject{container.NewCenter(e.empty)}
	}
	e.right.Refresh()
}

// newStructured is the starting point of every new preset: H.264 on CPU by quality, audio copied.
func newStructured(id string) preset.Preset {
	return preset.Preset{
		ID: id, Name: "Новый пресет", Suffix: "new", Ext: "mp4",
		Video: &preset.Video{Codec: preset.CodecH264, Encoder: preset.EncCPU, RateMode: preset.RateQuality, Quality: 20, Speed: "medium"},
		Audio: &preset.Audio{Mode: preset.AudioCopy},
	}
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

func (e *editor) build() {
	e.list = widget.NewList(
		func() int { return len(e.cfg.Presets) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil, widget.NewCheck("", nil), nil, widget.NewLabel("preset"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*fyne.Container)
			lbl := row.Objects[0].(*widget.Label)
			chk := row.Objects[1].(*widget.Check)
			p := e.cfg.Presets[i]
			lbl.SetText(p.Name)
			lbl.Importance = widget.MediumImportance
			if p.Disabled {
				lbl.Importance = widget.LowImportance
			}
			lbl.Refresh()
			chk.OnChanged = nil
			chk.SetChecked(!p.Disabled)
			chk.OnChanged = func(on bool) {
				e.cfg.Presets[i].Disabled = !on
				e.list.RefreshItem(i)
				e.markDirty()
			}
		},
	)
	e.list.OnSelected = func(i widget.ListItemID) { e.sel = i; e.showForm(true); e.loadForm() }

	// --- list toolbar (icon buttons) ---
	icon := func(ic fyne.Resource, f func()) *widget.Button {
		b := widget.NewButtonWithIcon("", ic, f)
		b.Importance = widget.LowImportance
		return b
	}
	add := icon(theme.ContentAddIcon(), func() {
		e.cfg.Presets = append(e.cfg.Presets, newStructured(e.freeID("new")))
		e.list.Refresh()
		e.list.Select(len(e.cfg.Presets) - 1)
		e.markDirty()
	})
	dup := icon(theme.ContentCopyIcon(), func() {
		c := e.cur()
		if c == nil {
			return
		}
		n := clonePreset(*c)
		n.ID = e.freeID(c.ID + "-copy")
		n.Name += " (копия)"
		e.cfg.Presets = append(e.cfg.Presets, n)
		e.list.Refresh()
		e.list.Select(len(e.cfg.Presets) - 1)
		e.markDirty()
	})
	del := icon(theme.DeleteIcon(), func() {
		c := e.cur()
		if c == nil {
			return
		}
		dialog.ShowConfirm("Удалить пресет", "Удалить «"+c.Name+"»?", func(ok bool) {
			if !ok {
				return
			}
			i := e.sel
			e.cfg.Presets = append(e.cfg.Presets[:i], e.cfg.Presets[i+1:]...)
			e.sel = -1
			e.list.UnselectAll()
			e.list.Refresh()
			if len(e.cfg.Presets) == 0 {
				e.showForm(false)
			} else {
				if i >= len(e.cfg.Presets) {
					i = len(e.cfg.Presets) - 1
				}
				e.list.Select(i)
			}
			e.markDirty()
		}, e.win)
	})
	move := func(d int) func() {
		return func() {
			i, j := e.sel, e.sel+d
			if i < 0 || j < 0 || j >= len(e.cfg.Presets) {
				return
			}
			ps := e.cfg.Presets
			ps[i], ps[j] = ps[j], ps[i]
			e.list.Refresh()
			e.list.Select(j)
			e.markDirty()
		}
	}
	up := icon(theme.MoveUpIcon(), move(-1))
	down := icon(theme.MoveDownIcon(), move(1))
	listBar := container.NewHBox(add, dup, del, layoutSpacer(), up, down)
	left := container.NewBorder(
		container.NewVBox(widget.NewLabelWithStyle("Пресеты (галочка = пункт в меню)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), listBar),
		nil, nil, nil, e.list)

	// --- general fields (shared by both kinds) ---
	mk := func(f func(*preset.Preset, string)) *widget.Entry {
		en := widget.NewEntry()
		en.OnChanged = func(s string) {
			if e.loading {
				return
			}
			if c := e.cur(); c != nil {
				f(c, s)
				e.refreshPreview()
				e.markDirty()
			}
		}
		return en
	}
	e.name = mk(func(p *preset.Preset, s string) {
		if p.ID == Slug(p.Name) || p.ID == "" {
			p.ID = Slug(s)
			e.loading = true
			e.id.SetText(p.ID)
			e.loading = false
		}
		p.Name = s
		e.list.Refresh()
	})
	e.id = mk(func(p *preset.Preset, s string) { p.ID = s })
	e.suffix = mk(func(p *preset.Preset, s string) { p.Suffix = s })
	e.name.SetPlaceHolder("Как пункт называется в меню")
	e.id.SetPlaceHolder("a-z, 0-9, дефис")
	e.suffix.SetPlaceHolder("fhd → имя.fhd.mp4")
	e.ext = widget.NewSelectEntry(append(append([]string{}, extOptions...), "m4v", "avi", "mp3", "m4a", "wav"))
	e.ext.SetPlaceHolder("same = как у источника")
	e.ext.OnChanged = func(s string) {
		if e.loading {
			return
		}
		if c := e.cur(); c != nil {
			c.Ext = s
			e.refreshPreview()
			e.markDirty()
		}
	}
	general := widget.NewCard("Основное", "", widget.NewForm(
		widget.NewFormItem("Название", e.name),
		widget.NewFormItem("id", e.id),
		widget.NewFormItem("Суффикс", e.suffix),
		widget.NewFormItem("Контейнер", e.ext),
	))

	// --- raw (legacy) fields ---
	rawMk := func(f func(*preset.Preset, string)) *widget.Entry {
		return mk(f)
	}
	e.rawVF = rawMk(func(p *preset.Preset, s string) { p.VF = s })
	e.rawAF = rawMk(func(p *preset.Preset, s string) { p.AF = s })
	e.rawArgs = rawMk(func(p *preset.Preset, s string) {
		if a, err := preset.SplitArgs(s); err == nil {
			p.Args = a
		}
	})
	e.rawVF.SetPlaceHolder("например scale=1920:1080,setsar=1")
	e.rawAF.SetPlaceHolder("например volume=-6dB")
	e.rawArgs.SetPlaceHolder("-c:v libx264 -crf 18 -c:a copy")
	note := widget.NewLabel("Это пресет из сырых аргументов ffmpeg (старый формат). Он работает как раньше. " +
		"Чтобы редактировать его в конструкторе, импортируйте его команду.")
	note.Wrapping = fyne.TextWrapWord
	convert := widget.NewButtonWithIcon("Открыть в конструкторе (через импорт)…", theme.DocumentCreateIcon(), e.convertLegacy)
	e.rawCard = widget.NewCard("Сырые аргументы", "", container.NewVBox(note,
		widget.NewForm(
			widget.NewFormItem("Видеофильтр (vf)", e.rawVF),
			widget.NewFormItem("Аудиофильтр (af)", e.rawAF),
			widget.NewFormItem("Аргументы", e.rawArgs),
		), convert))

	// --- builder (structured presets) ---
	e.b = newBuilder(e)
	importBtn := widget.NewButtonWithIcon("Импорт из команды ffmpeg…", theme.DownloadIcon(), func() { e.importDialog("") })
	builderHead := container.NewHBox(widget.NewLabelWithStyle("Конструктор", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layoutSpacer(), importBtn)

	// --- issues + preview ---
	e.issuesBox = container.NewVBox()
	e.preview = widget.NewLabel("")
	e.preview.Wrapping = fyne.TextWrapBreak
	e.preview.TextStyle.Monospace = true
	copyBtn := widget.NewButtonWithIcon("Копировать", theme.ContentCopyIcon(), func() {
		if e.previewText != "" {
			fyne.CurrentApp().Clipboard().SetContent(e.previewText)
		}
	})
	cmdCard := widget.NewCard("Итоговая команда", "", container.NewBorder(nil, container.NewHBox(layoutSpacer(), copyBtn), nil, nil, e.preview))

	e.form = container.NewVScroll(container.NewVBox(general, e.rawCard, builderHead, e.b.cards, e.issuesBox, cmdCard))
	e.empty = widget.NewLabel("Нет пресетов. Нажмите «+» слева.")
	e.right = container.NewStack()

	// --- top bar ---
	e.saveBtn = widget.NewButtonWithIcon("Сохранить", theme.DocumentSaveIcon(), e.save)
	e.saveBtn.Importance = widget.HighImportance
	e.revertBtn = widget.NewButtonWithIcon("Сбросить правки", theme.ContentUndoIcon(), e.revert)
	test := widget.NewButtonWithIcon("Тест на файле…", theme.MediaPlayIcon(), e.test)
	e.dirtyLbl = widget.NewLabel("")
	top := container.NewVBox(
		container.NewHBox(e.saveBtn, e.revertBtn, widget.NewSeparator(), test, layoutSpacer(), e.dirtyLbl),
		widget.NewSeparator())

	e.status = widget.NewLabel("")
	bottom := container.NewVBox(widget.NewSeparator(), e.status)

	split := container.NewHSplit(left, e.right)
	split.Offset = 0.26
	e.win.SetContent(container.NewBorder(top, bottom, nil, nil, split))

	e.win.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("Меню Windows",
			fyne.NewMenuItem("Установить меню (по сохранённым пресетам)", e.installMenu),
			fyne.NewMenuItem("Удалить меню", e.uninstallMenu),
			fyne.NewMenuItem("Удалить сертификат из доверенных (UAC)", e.untrustCert),
		),
		fyne.NewMenu("Настройки", fyne.NewMenuItem("Параметры…", e.settings)),
	))
}

func (e *editor) freeID(base string) string {
	id := base
	for n := 2; e.hasID(id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func (e *editor) hasID(id string) bool {
	for _, p := range e.cfg.Presets {
		if p.ID == id {
			return true
		}
	}
	return false
}

// loadForm fills the form from the selected preset and switches between builder and raw mode.
func (e *editor) loadForm() {
	c := e.cur()
	if c == nil {
		return
	}
	e.loading = true
	e.name.SetText(c.Name)
	e.id.SetText(c.ID)
	e.suffix.SetText(c.Suffix)
	e.ext.SetText(c.Ext)
	if c.Structured() {
		e.rawCard.Hide()
		e.b.cards.Show()
		e.b.load(*c)
	} else {
		e.b.cards.Hide()
		e.rawCard.Show()
		e.rawVF.SetText(c.VF)
		e.rawAF.SetText(c.AF)
		e.rawArgs.SetText(preset.JoinArgs(c.Args))
	}
	e.loading = false
	e.refreshPreview()
}

// encReport returns the encoder availability report and whether the background probe has finished.
func (e *editor) encReport() (encoders.Report, bool) {
	r, ready, _ := e.enc.Report()
	return r, ready
}

// refreshPreview rebuilds the issue list and the live command preview for the selected preset.
func (e *editor) refreshPreview() {
	c := e.cur()
	if c == nil {
		return
	}
	var issues []cmdbuild.Issue
	if c.Structured() {
		if _, err := preset.SplitArgs(e.b.args.Text); err != nil {
			issues = append(issues, cmdbuild.Issue{Severity: cmdbuild.Error, Msg: "Дополнительные аргументы: " + err.Error()})
		}
		issues = append(issues, cmdbuild.Validate(*c)...)
		if rep, ready := e.encReport(); ready {
			issues = append(issues, cmdbuild.CheckAvailability(*c, rep)...)
		}
	} else if _, err := preset.SplitArgs(e.rawArgs.Text); err != nil {
		issues = append(issues, cmdbuild.Issue{Severity: cmdbuild.Error, Msg: "Аргументы: " + err.Error()})
	}
	e.showIssues(issues)

	ext := strings.Replace(c.Ext, "same", "mp4", 1)
	out := "output." + c.Suffix + "." + ext
	var lines []string
	if cmdbuild.NeedsTwoPass(*c) {
		for pass := 1; pass <= 2; pass++ {
			if a, err := cmdbuild.Args(*c, "input.mp4", out, cmdbuild.Opts{SrcExt: ".mp4", Preview: true, Pass: pass, PassLog: "pass"}); err == nil {
				lines = append(lines, fmt.Sprintf("# проход %d из 2", pass), "ffmpeg "+preset.JoinArgs(a))
			}
		}
	} else if a, err := cmdbuild.Args(*c, "input.mp4", out, cmdbuild.Opts{SrcExt: ".mp4", Preview: true}); err == nil {
		lines = append(lines, "ffmpeg "+preset.JoinArgs(a))
	}
	if len(lines) == 0 {
		e.previewText = ""
		e.preview.SetText("Команду собрать нельзя: исправьте ошибки выше.")
		return
	}
	e.previewText = strings.Join(lines, "\n")
	e.preview.SetText(e.previewText)
}

func (e *editor) showIssues(is []cmdbuild.Issue) {
	e.issuesBox.Objects = nil
	for _, i := range is {
		l := widget.NewLabel("")
		l.Wrapping = fyne.TextWrapWord
		if i.Severity == cmdbuild.Error {
			l.Importance = widget.DangerImportance
			l.SetText("✖ " + i.Msg)
		} else {
			l.Importance = widget.WarningImportance
			l.SetText("⚠ " + i.Msg)
		}
		e.issuesBox.Add(l)
	}
	e.issuesBox.Refresh()
}

// startProbe checks the encoders in the background (cached; force = «Перепроверить»).
func (e *editor) startProbe(force bool) {
	e.status.SetText("Проверка ffmpeg и энкодеров…")
	path := e.cfg.Settings.FFmpegPath
	go func() {
		t, err := runner.FindTools(path)
		if err != nil {
			fyne.Do(func() { e.ffVersion = ""; e.status.SetText("ffmpeg: не найден") })
			return
		}
		ver := t.Version()
		fyne.Do(func() { e.ffVersion = ver })
		e.enc.Refresh(t.FFmpeg, force, func(_ encoders.Report, err error) {
			fyne.Do(func() { e.onProbeDone(err) })
		})
	}()
}

func (e *editor) onProbeDone(err error) {
	e.updateStatus(err)
	if c := e.cur(); c != nil && c.Structured() {
		e.b.refreshEncoders(*c)
		e.refreshPreview()
	}
}

// updateStatus renders the status bar from the ffmpeg version and the encoder report.
func (e *editor) updateStatus(probeErr error) {
	s := e.ffVersion
	if s == "" {
		s = "ffmpeg"
	}
	switch rep, ready := e.encReport(); {
	case probeErr != nil:
		s += " · не удалось проверить энкодеры: " + probeErr.Error()
	case !ready:
		s += " · проверка энкодеров…"
	default:
		var hw []string
		for _, f := range familyNames[1:] {
			var codecs []string
			for _, c := range codecOpts[1:] {
				if rep.OK(cmdbuild.EncoderName(c.val, f.val)) {
					codecs = append(codecs, strings.Fields(c.label)[0])
				}
			}
			if len(codecs) > 0 {
				hw = append(hw, fmt.Sprintf("%s (%s)", f.label, strings.Join(codecs, ", ")))
			}
		}
		if len(hw) == 0 {
			s += " · аппаратных энкодеров нет"
		} else {
			s += " · аппаратные: " + strings.Join(hw, ", ")
		}
	}
	if e.hooks.Installed != nil && e.hooks.Installed() {
		s += " · меню установлено"
	} else {
		s += " · меню не установлено"
	}
	e.status.SetText(s)
}

// validate checks every preset before saving; errors block the save, warnings do not.
func (e *editor) validate() error {
	if err := preset.ValidateAll(e.cfg.Presets); err != nil {
		return err
	}
	for _, p := range e.cfg.Presets {
		for _, is := range cmdbuild.Validate(p) {
			if is.Severity == cmdbuild.Error {
				return fmt.Errorf("пресет «%s»: %s", p.Name, is.Msg)
			}
		}
	}
	if c := e.cur(); c != nil {
		var args string
		if c.Structured() {
			args = e.b.args.Text
		} else {
			args = e.rawArgs.Text
		}
		if _, err := preset.SplitArgs(args); err != nil {
			return fmt.Errorf("пресет «%s»: аргументы: %w", c.Name, err)
		}
	}
	return nil
}

func (e *editor) save() {
	if err := e.validate(); err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	if err := preset.Save(e.path, e.cfg); err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	e.snapshot()
	e.markDirty()
	if err := e.hooks.Install(e.cfg); err != nil {
		dialog.ShowError(fmt.Errorf("пресеты сохранены, но меню не обновлено: %w", err), e.win)
	}
	e.updateStatus(nil)
}

// installMenu registers the menu from the saved presets.toml (not unsaved edits).
func (e *editor) installMenu() {
	c, err := preset.Load(e.path)
	if err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	if err := e.hooks.Install(c); err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	e.updateStatus(nil)
	dialog.ShowInformation("Меню", "Меню установлено по сохранённым пресетам.", e.win)
}

func (e *editor) uninstallMenu() {
	dialog.ShowConfirm("Удалить меню", "Удалить пункты контекстного меню vidpreset?", func(ok bool) {
		if !ok {
			return
		}
		if err := e.hooks.Uninstall(); err != nil {
			dialog.ShowError(err, e.win)
			return
		}
		e.updateStatus(nil)
	}, e.win)
}

func (e *editor) untrustCert() {
	dialog.ShowConfirm("Сертификат", "Удалить сертификат vidpreset из доверенных? Новое меню Windows 11 нельзя будет установить, пока он не будет добавлен снова.", func(ok bool) {
		if !ok {
			return
		}
		if e.hooks.Untrust == nil {
			return
		}
		if err := e.hooks.Untrust(); err != nil {
			dialog.ShowError(err, e.win)
		}
	}, e.win)
}

// revert drops unsaved edits and reloads presets.toml from disk.
func (e *editor) revert() {
	dialog.ShowConfirm("Сбросить правки",
		"Отбросить все несохранённые изменения и вернуть пресеты из файла presets.toml?",
		func(ok bool) {
			if !ok {
				return
			}
			c, err := preset.Load(e.path)
			if err != nil {
				dialog.ShowError(err, e.win)
				return
			}
			e.cfg = c
			e.snapshot()
			e.sel = -1
			e.list.UnselectAll()
			e.list.Refresh()
			if len(c.Presets) > 0 {
				e.list.Select(0)
			} else {
				e.showForm(false)
			}
			e.markDirty()
		}, e.win)
}

// test runs the current (possibly unsaved) preset on a chosen file, with the chosen encoder and rate mode.
func (e *editor) test() {
	c := e.cur()
	if c == nil {
		return
	}
	if err := c.Validate(); err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	for _, is := range cmdbuild.Validate(*c) {
		if is.Severity == cmdbuild.Error {
			dialog.ShowError(fmt.Errorf("%s", is.Msg), e.win)
			return
		}
	}
	p := clonePreset(*c)
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		path := r.URI().Path()
		r.Close()
		e.hooks.Test(p, path, e.cfg)
	}, e.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".mp4", ".mkv", ".mov", ".avi", ".webm", ".m4v", ".ts"}))
	d.Show()
}

// --- import from an ffmpeg command ---

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
	section("Распознано (попадёт в форму):", r.Recognized)
	section("Останется в дополнительных аргументах (сырое):", r.Extra)
	section("Предупреждения:", r.Warnings)
	section("Не используется (ffmpeg, входной и выходной файлы, служебные флаги):", r.Ignored)
	return strings.TrimSpace(b.String())
}

func (e *editor) importDialog(initial string) {
	if e.cur() == nil {
		return
	}
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapWord
	entry.SetMinRowsVisible(5)
	entry.SetPlaceHolder(`ffmpeg -i input.mp4 -vf "scale=1920:1080,setsar=1" -c:v libx264 -crf 18 -preset medium -c:a copy output.mp4`)
	report := widget.NewLabel("Вставьте команду ffmpeg: ниже появится, что распознано, а что останется сырым.")
	report.Wrapping = fyne.TextWrapWord
	var res *ffimport.Result
	update := func(s string) {
		if strings.TrimSpace(s) == "" {
			res = nil
			report.SetText("Вставьте команду ffmpeg: ниже появится, что распознано, а что останется сырым.")
			return
		}
		r, err := ffimport.Parse(s)
		if err != nil {
			res = nil
			report.SetText("✖ " + err.Error())
			return
		}
		res = &r
		report.SetText(formatImport(r))
	}
	entry.OnChanged = update
	content := container.NewBorder(entry, nil, nil, nil, container.NewVScroll(report))
	d := dialog.NewCustomConfirm("Импорт из команды ffmpeg", "Применить к пресету", "Отмена", content, func(ok bool) {
		if !ok {
			return
		}
		if res == nil {
			dialog.ShowError(fmt.Errorf("нечего применять: команда не разобрана"), e.win)
			return
		}
		e.applyImport(*res)
	}, e.win)
	d.Resize(fyne.NewSize(760, 580))
	d.Show()
	if initial != "" {
		entry.SetText(initial)
	}
}

// applyImport replaces the encoding settings of the selected preset; name, id and suffix stay.
func (e *editor) applyImport(r ffimport.Result) {
	c := e.cur()
	if c == nil {
		return
	}
	c.Video, c.Audio = r.Preset.Video, r.Preset.Audio
	c.VF, c.AF, c.Args = r.Preset.VF, r.Preset.AF, r.Preset.Args
	if r.Preset.Ext != "" {
		c.Ext = r.Preset.Ext
	}
	e.loadForm()
	e.markDirty()
}

// convertLegacy opens the import dialog prefilled with the raw preset's command.
func (e *editor) convertLegacy() {
	c := e.cur()
	if c == nil {
		return
	}
	a, err := cmdbuild.Args(*c, "input.mp4", "output.mp4", cmdbuild.Opts{})
	if err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	e.importDialog("ffmpeg " + preset.JoinArgs(a))
}

func (e *editor) settings() {
	mode := widget.NewRadioGroup([]string{"Подменю", "Один пункт с окном выбора"}, nil)
	if e.cfg.Settings.MenuMode == "single" {
		mode.SetSelected("Один пункт с окном выбора")
	} else {
		mode.SetSelected("Подменю")
	}
	intg := widget.NewRadioGroup([]string{"Новое меню Windows 11", "Классическое (Windows 10 / «Показать дополнительные параметры»)"}, nil)
	if e.cfg.Settings.Integration == "classic" {
		intg.SetSelected("Классическое (Windows 10 / «Показать дополнительные параметры»)")
	} else {
		intg.SetSelected("Новое меню Windows 11")
	}
	auto := widget.NewCheck("Закрывать окно прогресса после успеха", nil)
	auto.SetChecked(e.cfg.Settings.AutoClose)
	ff := widget.NewEntry()
	ff.SetPlaceHolder("пусто = искать в PATH")
	ff.SetText(e.cfg.Settings.FFmpegPath)
	recheck := widget.NewButtonWithIcon("Перепроверить энкодеры", theme.ViewRefreshIcon(), func() {
		e.cfg.Settings.FFmpegPath = strings.TrimSpace(ff.Text)
		e.startProbe(true)
	})
	dialog.ShowForm("Параметры", "OK", "Отмена", []*widget.FormItem{
		widget.NewFormItem("Интеграция", intg),
		widget.NewFormItem("Меню", mode),
		widget.NewFormItem("", auto),
		widget.NewFormItem("Путь к ffmpeg", ff),
		widget.NewFormItem("Энкодеры", recheck),
	}, func(ok bool) {
		if !ok {
			return
		}
		e.cfg.Settings.MenuMode = "submenu"
		if mode.Selected == "Один пункт с окном выбора" {
			e.cfg.Settings.MenuMode = "single"
		}
		e.cfg.Settings.Integration = "modern"
		if intg.Selected != "Новое меню Windows 11" {
			e.cfg.Settings.Integration = "classic"
		}
		e.cfg.Settings.AutoClose = auto.Checked
		e.cfg.Settings.FFmpegPath = strings.TrimSpace(ff.Text)
		e.markDirty()
		e.startProbe(false)
	}, e.win)
}
