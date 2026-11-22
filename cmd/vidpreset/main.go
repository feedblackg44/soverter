package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"vidpreset/internal/ipc"
	"vidpreset/internal/preset"
	"vidpreset/internal/runner"
	"vidpreset/internal/shellext"
	"vidpreset/internal/ui"
)

func main() {
	args := os.Args[1:]
	cmd := "edit"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "edit":
		cmdEdit()
	case "run":
		if len(args) < 2 {
			fatal("использование: vidpreset run <id> <файл>...")
		}
		cmdRun(args[0], args[1:])
	case "pick":
		if len(args) < 1 {
			fatal("использование: vidpreset pick <файл>...")
		}
		cmdPick(args)
	case "install":
		attachConsole()
		cfg := mustLoad()
		check(installMenu(cfg))
		fmt.Println("Меню установлено.")
	case "uninstall":
		attachConsole()
		check(uninstallMenu())
		if len(args) > 0 && args[0] == "--cert" {
			check(shellext.UntrustCert())
		}
		fmt.Println("Меню удалено.")
	case "list":
		attachConsole()
		cfg := mustLoad()
		for _, p := range cfg.Presets {
			fmt.Printf("%-14s %-28s .%s.%s\n", p.ID, p.Name, p.Suffix, p.Ext)
		}
	default:
		fatal("неизвестная команда: " + cmd + "\nдоступно: edit, run, pick, install, uninstall, list")
	}
}

func check(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

func mustLoad() *preset.Config {
	p, err := preset.Path()
	check(err)
	c, err := preset.Load(p)
	check(err)
	return c
}

// session owns the single progress window of this process.
type session struct {
	app  fyne.App
	cfg  *preset.Config
	prog *ui.Progress
}

func (s *session) ensure() error {
	if s.prog != nil {
		return nil
	}
	tools, err := runner.FindTools(s.cfg.Settings.FFmpegPath)
	if err != nil {
		return err
	}
	s.prog = ui.NewProgress(s.app, tools, s.cfg.Settings.AutoClose, func() { s.prog = nil })
	return nil
}

func (s *session) add(p preset.Preset, files []string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	s.prog.Add(p, files...)
	return nil
}

// become either takes ownership of the single-instance port (returns listener)
// or forwards the job to the current owner (returns nil, done=true).
func forwardOrOwn(id string, files []string) (l net.Listener, forwarded bool) {
	l, err := ipc.Listen()
	if err == nil {
		return l, false
	}
	if err := ipc.Send(ipc.Msg{Preset: id, Files: files}); err != nil {
		fatal("не удалось передать задание уже запущенному окну: " + err.Error())
	}
	return nil, true
}

func cmdRun(id string, files []string) {
	cfg := mustLoad()
	p, ok := cfg.Find(id)
	if !ok {
		fatal("пресет не найден: " + id)
	}
	l, forwarded := forwardOrOwn(id, files)
	if forwarded {
		return
	}
	defer l.Close()
	a := app.NewWithID("vidpreset.run")
	s := &session{app: a, cfg: cfg}
	if err := s.add(p, files); err != nil {
		fatal(err.Error())
	}
	go ipc.Serve(l, func(m ipc.Msg) {
		if q, ok := cfg.Find(m.Preset); ok {
			fyne.Do(func() { _ = s.add(q, m.Files) })
		}
	})
	a.Run()
}

func cmdPick(files []string) {
	cfg := mustLoad()
	a := app.NewWithID("vidpreset.pick")
	s := &session{app: a, cfg: cfg}
	ui.Picker(a, preset.Enabled(cfg.Presets), func(p preset.Preset) {
		l, forwarded := forwardOrOwn(p.ID, files)
		if forwarded {
			a.Quit()
			return
		}
		if err := s.add(p, files); err != nil {
			l.Close()
			fatal(err.Error())
		}
		go ipc.Serve(l, func(m ipc.Msg) {
			if q, ok := cfg.Find(m.Preset); ok {
				fyne.Do(func() { _ = s.add(q, m.Files) })
			}
		})
	}, func() { a.Quit() })
	a.Run()
}

func cmdEdit() {
	p, err := preset.Path()
	check(err)
	cfg, err := preset.Load(p)
	check(err)
	a := app.NewWithID("vidpreset.edit")
	s := &session{app: a, cfg: cfg}
	ui.Editor(a, p, cfg, ui.EditorHooks{
		Install:   installMenu,
		Uninstall: uninstallMenu,
		Untrust:   shellext.UntrustCert,
		Installed: menuInstalled,
		Test: func(pr preset.Preset, file string, c *preset.Config) {
			s.cfg = c
			if err := s.add(pr, []string{file}); err != nil {
				showError(err.Error())
			}
		},
	})
	a.Run()
}

var _ = strings.TrimSpace
