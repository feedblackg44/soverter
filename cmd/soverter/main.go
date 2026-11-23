package main

import (
	"fmt"
	"net"
	"os"
	"soverter/internal/i18n"
	"sync"

	"soverter/internal/gui"
	"soverter/internal/ipc"
	"soverter/internal/preset"
	"soverter/internal/runner"
	"soverter/internal/shellext"
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
			fatal(i18n.T("использование: soverter run <id> <файл>..."))
		}
		cmdRun(args[0], args[1:])
	case "pick":
		if len(args) < 1 {
			fatal(i18n.T("использование: soverter pick <файл>..."))
		}
		cmdPick(args)
	case "install":
		attachConsole()
		cfg := mustLoad()
		check(installMenu(cfg))
		fmt.Println(i18n.T("Меню установлено."))
	case "uninstall":
		attachConsole()
		check(uninstallMenu())
		if len(args) > 0 && args[0] == "--cert" {
			check(shellext.UntrustCert())
		}
		fmt.Println(i18n.T("Меню удалено."))
	case "list":
		attachConsole()
		cfg := mustLoad()
		for _, p := range cfg.Presets {
			fmt.Printf("%-14s %-28s .%s.%s\n", p.ID, p.Name, p.Suffix, p.Ext)
		}
	default:
		fatal(i18n.T("неизвестная команда: ") + cmd + i18n.T("\nдоступно: edit, run, pick, install, uninstall, list"))
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
	gui.Configure(c.Settings)
	return c
}

// session owns the single progress window of this process.
type session struct {
	mu   sync.Mutex
	cfg  *preset.Config
	prog *gui.ProgressView
}

func (s *session) add(p preset.Preset, files []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prog == nil {
		tools, err := runner.FindTools(s.cfg.Settings.FFmpegPath)
		if err != nil {
			return err
		}
		tools.Unavailable = hardwareCheck(tools.FFmpeg)
		s.prog = gui.OpenProgress(tools, s.cfg.Settings.AutoClose, func() {
			s.mu.Lock()
			s.prog = nil
			s.mu.Unlock()
		})
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
		fatal(i18n.T("не удалось передать задание уже запущенному окну: ") + err.Error())
	}
	return nil, true
}

func serve(l net.Listener, cfg *preset.Config, s *session) {
	go ipc.Serve(l, func(m ipc.Msg) {
		if q, ok := cfg.Find(m.Preset); ok {
			_ = s.add(q, m.Files)
		}
	})
}

func cmdRun(id string, files []string) {
	cfg := mustLoad()
	p, ok := cfg.Find(id)
	if !ok {
		fatal(i18n.T("пресет не найден: ") + id)
	}
	l, forwarded := forwardOrOwn(id, files)
	if forwarded {
		return
	}
	defer l.Close()
	s := &session{cfg: cfg}
	if err := s.add(p, files); err != nil {
		fatal(err.Error())
	}
	serve(l, cfg, s)
	gui.Main()
}

func cmdPick(files []string) {
	cfg := mustLoad()
	s := &session{cfg: cfg}
	gui.OpenPicker(preset.Enabled(cfg.Presets), func(p preset.Preset) {
		l, forwarded := forwardOrOwn(p.ID, files)
		if forwarded {
			return
		}
		if err := s.add(p, files); err != nil {
			l.Close()
			fatal(err.Error())
		}
		serve(l, cfg, s)
	})
	gui.Main()
}

func cmdEdit() {
	p, err := preset.Path()
	check(err)
	cfg, err := preset.Load(p)
	check(err)
	gui.Configure(cfg.Settings)
	s := &session{cfg: cfg}
	gui.NewEditor(p, cfg, gui.EditorHooks{
		Install:   installMenu,
		Uninstall: uninstallMenu,
		Untrust:   shellext.UntrustCert,
		Installed: menuInstalled,
		Test: func(pr preset.Preset, file string, c *preset.Config) {
			s.mu.Lock()
			s.cfg = c
			s.mu.Unlock()
			if err := s.add(pr, []string{file}); err != nil {
				showError(err.Error())
			}
		},
	})
	gui.Main()
}
