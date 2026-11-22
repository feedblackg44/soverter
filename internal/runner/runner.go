// Package runner builds ffmpeg commands, parses progress and runs jobs.
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"vidpreset/internal/cmdbuild"
	"vidpreset/internal/preset"
	"vidpreset/internal/sizecalc"
)

// BuildArgs returns ffmpeg arguments for display and legacy callers (nil if the preset cannot be built).
// Size-mode presets show a placeholder bitrate. Run uses cmdbuild.Args directly.
func BuildArgs(p preset.Preset, in, out string) []string {
	a, err := cmdbuild.Args(p, in, out, cmdbuild.Opts{SrcExt: filepath.Ext(in), Preview: true})
	if err != nil {
		return nil
	}
	return a
}

// OutputPath picks <dir>/<name>.<suffix>.<ext>, adding -1, -2 ... on collision.
// exists is injectable for tests (nil = os.Stat).
func OutputPath(in string, p preset.Preset, exists func(string) bool) string {
	if exists == nil {
		exists = func(s string) bool { _, err := os.Stat(s); return err == nil }
	}
	dir := filepath.Dir(in)
	base := filepath.Base(in)
	srcExt := filepath.Ext(base)
	name := strings.TrimSuffix(base, srcExt)
	ext := strings.TrimPrefix(p.Ext, ".")
	if ext == "same" {
		ext = strings.TrimPrefix(srcExt, ".")
	}
	cand := filepath.Join(dir, fmt.Sprintf("%s.%s.%s", name, p.Suffix, ext))
	for i := 1; exists(cand); i++ {
		cand = filepath.Join(dir, fmt.Sprintf("%s.%s-%d.%s", name, p.Suffix, i, ext))
	}
	return cand
}

// Progress is a snapshot parsed from `-progress pipe:1` output.
type Progress struct {
	OutTime time.Duration
	Speed   float64 // 0 if unknown
	Done    bool
	// Pass/Passes are set (1/2, 2/2) while a two-pass encode is running.
	Pass, Passes int
}

// ParseProgressLine updates p from one key=value line. Returns true when a block ends (progress=...).
func ParseProgressLine(line string, p *Progress) bool {
	k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
	if !ok {
		return false
	}
	switch k {
	case "out_time_us", "out_time_ms": // both are microseconds in ffmpeg
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			p.OutTime = time.Duration(n) * time.Microsecond
		}
	case "speed":
		v = strings.TrimSuffix(strings.TrimSpace(v), "x")
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			p.Speed = f
		}
	case "progress":
		p.Done = v == "end"
		return true
	}
	return false
}

// ETA estimates remaining time; ok=false if unknown.
func ETA(total time.Duration, p Progress) (time.Duration, bool) {
	if total <= 0 || p.Speed <= 0 {
		return 0, false
	}
	rem := total - p.OutTime
	if rem < 0 {
		rem = 0
	}
	return time.Duration(float64(rem) / p.Speed), true
}

// Tools locates ffmpeg/ffprobe.
type Tools struct{ FFmpeg, FFprobe string }

func FindTools(manual string) (Tools, error) {
	ff := manual
	if ff == "" {
		p, err := exec.LookPath("ffmpeg")
		if err != nil {
			return Tools{}, errors.New("ffmpeg не найден в PATH.\nУстановите: winget install Gyan.FFmpeg")
		}
		ff = p
	} else if _, err := os.Stat(ff); err != nil {
		return Tools{}, fmt.Errorf("ffmpeg не найден по пути %s", ff)
	}
	probe := filepath.Join(filepath.Dir(ff), "ffprobe"+filepath.Ext(ff))
	if _, err := os.Stat(probe); err != nil {
		if p, err := exec.LookPath("ffprobe"); err == nil {
			probe = p
		} else {
			probe = ""
		}
	}
	return Tools{FFmpeg: ff, FFprobe: probe}, nil
}

// Duration asks ffprobe for the file duration; 0 if unknown.
func (t Tools) Duration(ctx context.Context, file string) time.Duration {
	if t.FFprobe == "" {
		return 0
	}
	cmd := exec.CommandContext(ctx, t.FFprobe, "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", file)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || f <= 0 {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

// Version returns the first line of `ffmpeg -version`.
func (t Tools) Version() string {
	cmd := exec.Command(t.FFmpeg, "-version")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	l, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(l)
}

var (
	encMu    sync.Mutex
	encCache = map[string]string{}
)

// HasEncoder checks `ffmpeg -encoders` for a named encoder. The (slow) listing is cached per ffmpeg path.
func (t Tools) HasEncoder(name string) bool {
	encMu.Lock()
	defer encMu.Unlock()
	list, ok := encCache[t.FFmpeg]
	if !ok {
		cmd := exec.Command(t.FFmpeg, "-hide_banner", "-encoders")
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		list = string(out)
		encCache[t.FFmpeg] = list
	}
	return encoderListed(list, name)
}

func encoderListed(list, name string) bool {
	for _, l := range strings.Split(list, "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && f[1] == name && len(f[0]) == 6 {
			return true
		}
	}
	return false
}

// RequiredEncoders extracts encoder names from -c:v/-c:a/-c/-vcodec/-acodec args (excluding "copy").
func RequiredEncoders(args []string) []string {
	var r []string
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "-c", "-c:v", "-c:a", "-c:s", "-vcodec", "-acodec", "-codec":
			if v := args[i+1]; v != "copy" {
				r = append(r, v)
			}
		}
	}
	return r
}

// CheckEncoders returns a friendly error if a required encoder is missing.
func (t Tools) CheckEncoders(p preset.Preset) error {
	needed := append(RequiredEncoders(p.Args), cmdbuild.Encoders(p)...)
	for _, e := range needed {
		if !t.HasEncoder(e) {
			return fmt.Errorf("в вашей сборке ffmpeg нет энкодера %q (нужен пресету «%s»).\nУстановите полную сборку: winget install Gyan.FFmpeg", e, p.Name)
		}
	}
	return nil
}

// ErrCanceled is returned by Run when ctx was canceled.
var ErrCanceled = errors.New("отменено")

// EncodeError carries ffmpeg's stderr tail.
type EncodeError struct {
	Err  error
	Tail string
}

func (e *EncodeError) Error() string { return fmt.Sprintf("ffmpeg: %v\n%s", e.Err, e.Tail) }

// ProbeMedia reads duration and audio info with one ffprobe call.
func (t Tools) ProbeMedia(ctx context.Context, file string) (sizecalc.Media, error) {
	if t.FFprobe == "" {
		return sizecalc.Media{}, errors.New("ffprobe не найден: режим «по размеру» недоступен")
	}
	cmd := exec.CommandContext(ctx, t.FFprobe, "-v", "error", "-show_entries",
		"format=duration,bit_rate:stream=codec_type,bit_rate", "-of", "json", file)
	hideWindow(cmd)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return sizecalc.Media{}, ErrCanceled
	}
	if err != nil {
		detail := err.Error()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if s := strings.TrimSpace(string(ee.Stderr)); s != "" {
				detail = s
			}
		}
		return sizecalc.Media{}, fmt.Errorf("ffprobe не смог прочитать файл (режим «по размеру» для него недоступен): %s", detail)
	}
	return sizecalc.ParseProbe(out)
}

// Run executes the preset for one file. The partial output is removed on failure or cancel.
// onProgress may be nil. Returns the output path.
//
// For rate_mode = size the video bitrate is computed from the file's duration and audio (ffprobe);
// libx264/libx265 then run in two passes with their log files in a temp directory that is always removed.
func (t Tools) Run(ctx context.Context, p preset.Preset, in string, onProgress func(Progress)) (string, error) {
	if err := t.CheckEncoders(p); err != nil {
		return "", err
	}
	out := OutputPath(in, p, nil)
	opts := cmdbuild.Opts{SrcExt: filepath.Ext(in)}

	if cmdbuild.NeedsSize(p) {
		media, err := t.ProbeMedia(ctx, in)
		if err != nil {
			return "", err
		}
		v, a := p.Normalized()
		kbps, err := sizecalc.VideoKbps(v.TargetSizeMB, media, a)
		if err != nil {
			return "", err
		}
		opts.VideoKbps = kbps
	}

	passes := []int{0}
	if cmdbuild.NeedsTwoPass(p) {
		dir, err := os.MkdirTemp("", "vidpreset-pass-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir) // logs are removed on success, failure and cancel
		opts.PassLog = filepath.Join(dir, "pass")
		passes = []int{1, 2}
	}

	for i, pass := range passes {
		o := opts
		o.Pass = pass
		built, err := cmdbuild.Args(p, in, out, o)
		if err != nil {
			return "", err
		}
		n := i + 1
		err = t.execFFmpeg(ctx, built, func(pr Progress) {
			if len(passes) > 1 {
				pr.Pass, pr.Passes = n, len(passes)
			}
			if onProgress != nil {
				onProgress(pr)
			}
		})
		if err != nil {
			_ = os.Remove(out)
			return "", err
		}
	}
	return out, nil
}

// execFFmpeg runs one ffmpeg process with progress reporting.
func (t Tools) execFFmpeg(ctx context.Context, built []string, onProgress func(Progress)) error {
	args := append([]string{"-nostats", "-progress", "pipe:1"}, built...)
	cmd := exec.CommandContext(ctx, t.FFmpeg, args...)
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	tail := &tailBuf{max: 20}
	cmd.Stderr = tail
	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return ErrCanceled
		}
		return err
	}
	var pr Progress
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if ParseProgressLine(sc.Text(), &pr) && onProgress != nil {
			onProgress(pr)
		}
	}
	werr := cmd.Wait()
	if ctx.Err() != nil {
		return ErrCanceled
	}
	if werr != nil {
		return &EncodeError{Err: werr, Tail: tail.String()}
	}
	return nil
}

type tailBuf struct {
	max   int
	lines []string
	part  string
}

func (b *tailBuf) Write(p []byte) (int, error) {
	s := b.part + strings.ReplaceAll(string(p), "\r", "\n")
	parts := strings.Split(s, "\n")
	b.part = parts[len(parts)-1]
	for _, l := range parts[:len(parts)-1] {
		if strings.TrimSpace(l) != "" {
			b.lines = append(b.lines, l)
		}
	}
	if len(b.lines) > b.max {
		b.lines = b.lines[len(b.lines)-b.max:]
	}
	return len(p), nil
}

func (b *tailBuf) String() string {
	l := b.lines
	if strings.TrimSpace(b.part) != "" {
		l = append(append([]string{}, l...), b.part)
	}
	return strings.Join(l, "\n")
}

// HideWindow makes a child process run without a console window (Windows).
func HideWindow(c *exec.Cmd) { hideWindow(c) }
