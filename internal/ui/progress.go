// Package ui contains the Fyne windows: progress, picker and editor.
package ui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"vidpreset/internal/preset"
	"vidpreset/internal/runner"
)

// Progress is the queue window shared by all `run` invocations.
type Progress struct {
	app       fyne.App
	win       fyne.Window
	q         *runner.Queue
	list      *fyne.Container
	autoClose bool
	cancel    *widget.Button
	onClose   func()
}

// NewProgress builds the window. onClose is called when the user closes it.
func NewProgress(a fyne.App, tools runner.Tools, autoClose bool, onClose func()) *Progress {
	p := &Progress{app: a, autoClose: autoClose, onClose: onClose}
	p.win = a.NewWindow("vidpreset")
	p.list = container.NewVBox()
	p.cancel = widget.NewButton("Отмена", func() { p.q.CancelCurrent() })
	p.q = runner.NewQueue(tools, func() { fyne.Do(p.refresh) })
	p.win.SetContent(container.NewBorder(nil, container.NewHBox(p.cancel), nil, nil, container.NewVScroll(p.list)))
	p.win.Resize(fyne.NewSize(560, 260))
	p.win.SetOnClosed(func() {
		p.q.CancelCurrent()
		if p.onClose != nil {
			p.onClose()
		}
	})
	p.win.Show()
	return p
}

// Add queues files with a preset. Safe to call from any goroutine.
func (p *Progress) Add(pr preset.Preset, files ...string) {
	fyne.Do(func() {
		p.win.Show()
	})
	p.q.Add(pr, files...)
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func (p *Progress) refresh() {
	items := p.q.Snapshot()
	p.list.Objects = nil
	failed := false
	for _, it := range items {
		it := it
		title := widget.NewLabel(fmt.Sprintf("%s — %s", filepath.Base(it.File), it.Preset.Name))
		title.TextStyle.Bold = true
		var row fyne.CanvasObject
		switch it.Status {
		case runner.Pending:
			row = container.NewVBox(title, widget.NewLabel("В очереди"))
		case runner.Running:
			var bar fyne.CanvasObject
			info := "Кодирование… " + fmtDur(it.Progress.OutTime)
			if it.Total > 0 {
				frac := float64(it.Progress.OutTime) / float64(it.Total)
				if frac > 1 {
					frac = 1
				}
				pb := widget.NewProgressBar()
				pb.SetValue(frac)
				bar = pb
				info = fmt.Sprintf("%.0f%%", frac*100)
				if it.Progress.Speed > 0 {
					info += fmt.Sprintf(" · %.2fx", it.Progress.Speed)
				}
				if eta, ok := runner.ETA(it.Total, it.Progress); ok {
					info += " · осталось " + fmtDur(eta)
				}
			} else {
				bar = widget.NewProgressBarInfinite()
			}
			row = container.NewVBox(title, bar, widget.NewLabel(info))
		case runner.Done:
			out := it.Output
			row = container.NewVBox(title, container.NewHBox(
				widget.NewLabel("Готово"),
				widget.NewButton("Показать в папке", func() { showInFolder(out) }),
			))
		case runner.Canceled:
			row = container.NewVBox(title, widget.NewLabel("Отменено"))
		case runner.Failed:
			failed = true
			msg := it.Err.Error()
			e := widget.NewLabel(msg)
			e.Wrapping = fyne.TextWrapWord
			row = container.NewVBox(title, e, widget.NewButton("Копировать", func() {
				fyne.CurrentApp().Clipboard().SetContent(msg)
			}))
		}
		p.list.Add(row)
		p.list.Add(widget.NewSeparator())
	}
	p.list.Refresh()
	idle := p.q.Idle()
	if idle {
		p.cancel.Disable()
	} else {
		p.cancel.Enable()
	}
	if idle && p.autoClose && !failed && len(items) > 0 {
		p.win.Close()
	}
}

func showInFolder(path string) {
	_ = exec.Command("explorer", "/select,"+path).Start()
}
