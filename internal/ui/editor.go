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

	list                         *widget.List
	name, id, suffix, vf, af, ar *widget.Entry
	ext                          *widget.SelectEntry
	preview, status, dirtyLbl    *widget.Label
	saveBtn, revertBtn           *widget.Button
	form                         fyne.CanvasObject
	empty                        *widget.Label
	right                        *fyne.Container
}

// Editor opens the preset editor window.
func Editor(a fyne.App, path string, cfg *preset.Config, hooks EditorHooks) {
	e := &editor{app: a, path: path, cfg: cfg, hooks: hooks, sel: -1}
	e.snapshot()
	e.win = a.NewWindow("vidpreset — редактор пресетов")
	e.build()
	e.win.Resize(fyne.NewSize(980, 600))
	e.win.SetCloseIntercept(e.onClose)
	e.win.Show()
	if len(e.cfg.Presets) > 0 {
		e.list.Select(0)
	} else {
		e.showForm(false)
	}
	e.markDirty()
	e.refreshStatus()
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
		e.cfg.Presets = append(e.cfg.Presets, preset.Preset{ID: e.freeID("new"), Name: "Новый пресет", Suffix: "new", Ext: "mp4", Args: []string{"-c:v", "libx264", "-crf", "20", "-c:a", "copy"}})
		e.list.Refresh()
		e.list.Select(len(e.cfg.Presets) - 1)
		e.markDirty()
	})
	dup := icon(theme.ContentCopyIcon(), func() {
		c := e.cur()
		if c == nil {
			return
		}
		n := *c
		n.Args = append([]string(nil), c.Args...)
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

	// --- form ---
	mk := func(f func(*preset.Preset, string)) *widget.Entry {
		en := widget.NewEntry()
		en.OnChanged = func(s string) {
			if e.loading {
				return
			}
			if c := e.cur(); c != nil {
				f(c, s)
				e.updatePreview()
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
	e.vf = mk(func(p *preset.Preset, s string) { p.VF = s })
	e.af = mk(func(p *preset.Preset, s string) { p.AF = s })
	e.ar = mk(func(p *preset.Preset, s string) {
		if a, err := preset.SplitArgs(s); err == nil {
			p.Args = a
		}
	})
	e.name.SetPlaceHolder("Как пункт называется в меню")
	e.id.SetPlaceHolder("a-z, 0-9, дефис")
	e.suffix.SetPlaceHolder("fhd → имя.fhd.mp4")
	e.vf.SetPlaceHolder("например scale=1920:1080,setsar=1")
	e.af.SetPlaceHolder("например volume=-6dB")
	e.ar.SetPlaceHolder("-c:v libx264 -crf 18 -c:a copy")
	e.ext = widget.NewSelectEntry([]string{"mp4", "mkv", "mov", "webm", "mp3", "m4a", "wav", "same"})
	e.ext.OnChanged = func(s string) {
		if e.loading {
			return
		}
		if c := e.cur(); c != nil {
			c.Ext = s
			e.updatePreview()
			e.markDirty()
		}
	}
	e.preview = widget.NewLabel("")
	e.preview.Wrapping = fyne.TextWrapBreak
	e.preview.TextStyle.Monospace = true

	general := widget.NewCard("Основное", "", widget.NewForm(
		widget.NewFormItem("Название", e.name),
		widget.NewFormItem("id", e.id),
		widget.NewFormItem("Суффикс", e.suffix),
		widget.NewFormItem("Расширение", e.ext),
	))
	e.ext.SetPlaceHolder("same = как у исходника")
	process := widget.NewCard("Обработка", "Видеофильтр, аудиофильтр и остальные аргументы ffmpeg (кавычки как в shell)", widget.NewForm(
		widget.NewFormItem("Видеофильтр (vf)", e.vf),
		widget.NewFormItem("Аудиофильтр (af)", e.af),
		widget.NewFormItem("Аргументы", e.ar),
	))
	cmdCard := widget.NewCard("Итоговая команда", "", e.preview)
	e.form = container.NewVScroll(container.NewVBox(general, process, cmdCard))
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
	split.Offset = 0.28
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
	e.vf.SetText(c.VF)
	e.af.SetText(c.AF)
	e.ar.SetText(preset.JoinArgs(c.Args))
	e.loading = false
	e.updatePreview()
}

func (e *editor) updatePreview() {
	c := e.cur()
	if c == nil {
		return
	}
	if _, err := preset.SplitArgs(e.ar.Text); err != nil {
		e.preview.SetText("args: " + err.Error())
		return
	}
	out := "input." + c.Suffix + "." + strings.Replace(c.Ext, "same", "mp4", 1)
	e.preview.SetText("ffmpeg " + preset.JoinArgs(runner.BuildArgs(*c, "input.mp4", out)))
}

// refreshStatus probes ffmpeg in the background so the window never blocks on it.
func (e *editor) refreshStatus() {
	e.status.SetText("Проверка ffmpeg…")
	path := e.cfg.Settings.FFmpegPath
	go func() {
		s := ""
		t, err := runner.FindTools(path)
		if err != nil {
			s = "ffmpeg: не найден"
		} else {
			yn := func(b bool) string {
				if b {
					return "есть"
				}
				return "нет"
			}
			s = fmt.Sprintf("%s · libx264: %s · libx265: %s", t.Version(), yn(t.HasEncoder("libx264")), yn(t.HasEncoder("libx265")))
		}
		if e.hooks.Installed != nil && e.hooks.Installed() {
			s += " · меню установлено"
		} else {
			s += " · меню не установлено"
		}
		fyne.Do(func() { e.status.SetText(s) })
	}()
}

func (e *editor) validate() error {
	if _, err := preset.SplitArgs(e.ar.Text); err != nil {
		return fmt.Errorf("args: %w", err)
	}
	return preset.ValidateAll(e.cfg.Presets)
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
	e.refreshStatus()
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
	e.refreshStatus()
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
		e.refreshStatus()
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

func (e *editor) test() {
	c := e.cur()
	if c == nil {
		return
	}
	if err := c.Validate(); err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	p := *c
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
	dialog.ShowForm("Параметры", "OK", "Отмена", []*widget.FormItem{
		widget.NewFormItem("Интеграция", intg),
		widget.NewFormItem("Меню", mode),
		widget.NewFormItem("", auto),
		widget.NewFormItem("Путь к ffmpeg", ff),
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
		e.refreshStatus()
	}, e.win)
}
