// Package encoders finds out which ffmpeg encoders really work on this machine.
//
// `ffmpeg -encoders` lists everything compiled in, including hardware encoders that cannot run without
// the matching GPU/driver. So software encoders are checked against that list, while every hardware
// candidate is verified with a tiny trial encode. Results are cached in memory and on disk.
package encoders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vidpreset/internal/runner"
)

// Hardware encoders that are verified by a trial encode.
var Hardware = []string{
	"h264_nvenc", "hevc_nvenc", "av1_nvenc",
	"h264_amf", "hevc_amf", "av1_amf",
	"h264_qsv", "hevc_qsv", "av1_qsv",
}

// Software encoders (and audio encoders) that are only checked against the encoder list.
var Software = []string{"libx264", "libx265", "libsvtav1", "aac", "libopus"}

// Status is the availability of one encoder.
type Status struct {
	Available bool   `json:"ok"`
	Reason    string `json:"reason,omitempty"` // user-facing explanation when unavailable
	Detail    string `json:"detail,omitempty"` // last line of ffmpeg's error, for tooltips
}

// Report maps encoder name to status.
type Report map[string]Status

// OK reports whether the encoder works; unknown names are treated as available
// (so a custom encoder in a raw preset is never blocked).
func (r Report) OK(name string) bool {
	s, known := r[name]
	return !known || s.Available
}

// Why returns the explanation for an unavailable encoder ("" if available).
func (r Report) Why(name string) string {
	if s, ok := r[name]; ok && !s.Available {
		return s.Reason
	}
	return ""
}

// ListContains reports whether `ffmpeg -encoders` output lists the encoder.
func ListContains(list, name string) bool {
	for _, l := range strings.Split(list, "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && f[1] == name && len(f[0]) == 6 {
			return true
		}
	}
	return false
}

// Reason explains why a hardware encoder that is in the build failed its trial encode.
func Reason(name string) string {
	switch {
	case strings.HasSuffix(name, "_nvenc"):
		return "нет видеокарты NVIDIA с поддержкой этого кодека или не установлен драйвер"
	case strings.HasSuffix(name, "_amf"):
		return "нет видеокарты AMD с поддержкой этого кодека или не установлен драйвер"
	case strings.HasSuffix(name, "_qsv"):
		return "нет графики Intel с поддержкой этого кодека или не установлен драйвер"
	}
	return "энкодер не запускается"
}

// Evaluate is the pure core: it combines the encoder list with trial results.
// probe is called (in parallel, at most `parallel` at a time) only for hardware encoders present in the list.
func Evaluate(list string, probe func(name string) error, parallel int) Report {
	rep := Report{}
	var mu sync.Mutex
	set := func(n string, s Status) { mu.Lock(); rep[n] = s; mu.Unlock() }

	for _, n := range Software {
		if ListContains(list, n) {
			set(n, Status{Available: true})
		} else {
			set(n, Status{Reason: "в вашей сборке ffmpeg нет этого энкодера (winget install Gyan.FFmpeg)"})
		}
	}
	if parallel < 1 {
		parallel = 1
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for _, n := range Hardware {
		if !ListContains(list, n) {
			set(n, Status{Reason: "в вашей сборке ffmpeg нет этого энкодера"})
			continue
		}
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := probe(n); err != nil {
				set(n, Status{Reason: Reason(n), Detail: lastLine(err.Error())})
			} else {
				set(n, Status{Available: true})
			}
		}(n)
	}
	wg.Wait()
	return rep
}

func lastLine(s string) string {
	var keep []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.Contains(l, "Nothing was written") {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return strings.TrimSpace(s)
	}
	return keep[len(keep)-1]
}

// trial runs a very short encode with the given encoder into the null muxer.
func trial(ctx context.Context, ffmpeg, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-v", "error", "-f", "lavfi", "-i", "color=s=256x256:d=0.1",
		"-c:v", name, "-pix_fmt", "yuv420p", "-f", "null", "-")
	runner.HideWindow(cmd)
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func listEncoders(ctx context.Context, ffmpeg string) (string, error) {
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-encoders")
	runner.HideWindow(cmd)
	out, err := cmd.Output()
	return string(out), err
}

// Probe checks every candidate (real ffmpeg). It uses the disk cache unless force is set.
func Probe(ctx context.Context, ffmpeg string, force bool) (Report, error) {
	key := CacheKey(ffmpeg)
	if !force {
		if r, ok := LoadCache(CachePath(), key, time.Now()); ok {
			return r, nil
		}
	}
	list, err := listEncoders(ctx, ffmpeg)
	if err != nil {
		return nil, err
	}
	rep := Evaluate(list, func(n string) error { return trial(ctx, ffmpeg, n) }, 4)
	_ = SaveCache(CachePath(), key, rep, time.Now())
	return rep, nil
}

// --- disk cache ---

// CacheTTL bounds how long a cached result is trusted (drivers and GPUs change).
const CacheTTL = 7 * 24 * time.Hour

type cacheFile struct {
	Key    string    `json:"key"`
	Time   time.Time `json:"time"`
	Report Report    `json:"report"`
}

// CacheKey identifies the ffmpeg build: path, size and modification time.
func CacheKey(ffmpeg string) string {
	st, err := os.Stat(ffmpeg)
	if err != nil {
		return ffmpeg
	}
	return fmt.Sprintf("%s|%d|%d", ffmpeg, st.Size(), st.ModTime().UnixNano())
}

func CachePath() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "vidpreset", "encoders.json")
}

// LoadCache returns the cached report if it matches key and is fresh.
func LoadCache(path, key string, now time.Time) (Report, bool) {
	if path == "" {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var c cacheFile
	if json.Unmarshal(b, &c) != nil || c.Key != key || now.Sub(c.Time) > CacheTTL || len(c.Report) == 0 {
		return nil, false
	}
	return c.Report, true
}

func SaveCache(path, key string, r Report, now time.Time) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(cacheFile{Key: key, Time: now, Report: r})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// --- background manager (used by the UI) ---

// Manager runs the probe once in the background and caches the result in memory.
type Manager struct {
	mu      sync.Mutex
	rep     Report
	ffmpeg  string
	running bool
}

// Report returns the current result; ready is false until the first probe finished.
func (m *Manager) Report() (r Report, ready, running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rep, m.rep != nil, m.running
}

// Refresh starts a background probe (no-op if one is running). With force the caches are ignored
// ("Перепроверить"). done is called from the worker goroutine when finished.
func (m *Manager) Refresh(ffmpeg string, force bool, done func(Report, error)) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	if !force && m.rep != nil && m.ffmpeg == ffmpeg {
		r := m.rep
		m.mu.Unlock()
		if done != nil {
			go done(r, nil)
		}
		return
	}
	m.running = true
	m.mu.Unlock()
	go func() {
		r, err := Probe(context.Background(), ffmpeg, force)
		m.mu.Lock()
		m.running = false
		if err == nil {
			m.rep, m.ffmpeg = r, ffmpeg
		}
		m.mu.Unlock()
		if done != nil {
			done(r, err)
		}
	}()
}
