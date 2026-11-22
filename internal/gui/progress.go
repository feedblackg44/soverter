package gui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"vidpreset/internal/i18n"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"

	"vidpreset/internal/gui/theme"
	"vidpreset/internal/preset"
	"vidpreset/internal/runner"
)

// ProgressView is the queue window shared by all `run` invocations.
type ProgressView struct {
	win       *Win
	q         *runner.Queue
	autoClose bool
	onClose   func()
	list      widget.List
	cancel    Btn
	acts      []Btn
	closed    bool
	mu        sync.Mutex
}

// OpenProgress opens the queue window. onClose is called when the user closes it.
func OpenProgress(tools runner.Tools, autoClose bool, onClose func()) *ProgressView {
	p := &ProgressView{autoClose: autoClose, onClose: onClose}
	p.list.Axis = layout.Vertical
	p.q = runner.NewQueue(tools, func() {
		if p.win != nil {
			p.win.Invalidate()
		}
	})
	p.win = Open(i18n.T("vidpreset — очередь"), 620, 420, p)
	return p
}

// Add queues files with a preset. Safe to call from any goroutine.
func (p *ProgressView) Add(pr preset.Preset, files ...string) { p.q.Add(pr, files...) }

// CanClose cancels the running job and notifies the owner.
func (p *ProgressView) CanClose() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		p.q.CancelCurrent()
		if p.onClose != nil {
			p.onClose()
		}
	}
	return true
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// Layout draws the queue.
func (p *ProgressView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	items := p.q.Snapshot()
	for len(p.acts) < len(items) {
		p.acts = append(p.acts, Btn{})
	}
	var rows []QueueRow
	failed := false
	for i, it := range items {
		it := it
		row := QueueRow{File: filepath.Base(it.File), Preset: it.Preset.Name, Frac: -1}
		switch it.Status {
		case runner.Pending:
			row.State, row.Info = "pending", i18n.T("В очереди")
		case runner.Running:
			row.State = "running"
			row.Info = i18n.T("Кодирование… ") + fmtDur(it.Progress.OutTime)
			if it.Total > 0 {
				frac := float64(it.Progress.OutTime) / float64(it.Total)
				if frac > 1 {
					frac = 1
				}
				row.Frac = frac
				row.Info = fmt.Sprintf("%.0f%%", frac*100)
				if it.Progress.Speed > 0 {
					row.Info += fmt.Sprintf(" · %.2fx", it.Progress.Speed)
				}
				if eta, ok := runner.ETA(it.Total, it.Progress); ok {
					row.Info += i18n.T(" · осталось ") + fmtDur(eta)
				}
			} else {
				row.Frac = 0
			}
			if it.Progress.Passes > 1 {
				row.Info = fmt.Sprintf(i18n.T("проход %d из %d · %s"), it.Progress.Pass, it.Progress.Passes, row.Info)
			}
		case runner.Done:
			row.State, row.Info = "done", i18n.T("Готово")
			row.Action, row.ActionLabel = &p.acts[i], i18n.T("Показать в папке")
			if p.acts[i].Clicked(gtx) {
				ShowInFolder(it.Output)
			}
		case runner.Canceled:
			row.State, row.Info = "canceled", i18n.T("Отменено")
		case runner.Failed:
			failed = true
			msg := it.Err.Error()
			row.State, row.Info = "failed", msg
			row.Action, row.ActionLabel = &p.acts[i], i18n.T("Копировать ошибку")
			if p.acts[i].Clicked(gtx) {
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(msg))})
			}
		}
		rows = append(rows, row)
	}
	idle := p.q.Idle()
	if idle && p.autoClose && !failed && len(items) > 0 {
		p.win.Close()
	}
	if p.cancel.Clicked(gtx) {
		p.q.CancelCurrent()
	}
	return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(rows) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return th.Text(gtx, 14, 400, th.P.TextMuted, i18n.T("Очередь пуста"))
					})
				}
				return p.list.List.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
					return rows[i].Layout(gtx, th)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if idle {
					gtx = gtx.Disabled()
				}
				return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.cancel.Layout(gtx, th, i18n.T("Отменить текущий"), Secondary)
						}))
				})
			}),
		)
	})
}
