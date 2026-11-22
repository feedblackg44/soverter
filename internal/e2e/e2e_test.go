// Package e2e holds tests that run the real ffmpeg on this machine (skipped when it is missing).
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vidpreset/internal/encoders"
	"vidpreset/internal/preset"
	"vidpreset/internal/runner"
)

var (
	toolsOnce sync.Once
	tools     runner.Tools
	toolsErr  error
	report    encoders.Report
)

func setup(t *testing.T) runner.Tools {
	t.Helper()
	toolsOnce.Do(func() {
		tools, toolsErr = runner.FindTools("")
		if toolsErr == nil {
			report, toolsErr = encoders.Probe(context.Background(), tools.FFmpeg, false)
		}
	})
	if toolsErr != nil || tools.FFprobe == "" {
		t.Skip("ffmpeg/ffprobe not available")
	}
	return tools
}

func needEncoder(t *testing.T, name string) {
	t.Helper()
	if !report.OK(name) || report.Why(name) != "" {
		t.Skipf("encoder %s unavailable on this machine: %s", name, report.Why(name))
	}
}

// genVideo makes a source with a detailed picture and a sine tone. withAudio=false gives a silent video.
func genVideo(t *testing.T, dir, name string, seconds int, withAudio bool) string {
	t.Helper()
	path := filepath.Join(dir, name)
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x480:rate=25:duration=" + strconv.Itoa(seconds)}
	if withAudio {
		args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:duration="+strconv.Itoa(seconds), "-c:a", "aac", "-b:a", "128k")
	}
	args = append(args, "-pix_fmt", "yuv420p", "-c:v", "libx264", "-crf", "20", path)
	if out, err := exec.Command(tools.FFmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("generate source: %v\n%s", err, out)
	}
	return path
}

func fileMB(t *testing.T, path string) float64 {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return float64(st.Size()) / (1024 * 1024)
}

func video(v preset.Video, a preset.Audio) preset.Preset {
	return preset.Preset{ID: "e2e", Name: "e2e", Suffix: "e2e", Ext: "mp4", Video: &v, Audio: &a}
}

func tempPassDirs() int {
	m, _ := filepath.Glob(filepath.Join(os.TempDir(), "vidpreset-pass-*"))
	return len(m)
}

func probeSize(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command(tools.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// --- size mode ---

func TestSizeModeCPUTwoPass(t *testing.T) {
	tl := setup(t)
	needEncoder(t, "libx264")
	dir := t.TempDir()
	src := genVideo(t, dir, "src.mp4", 20, true)
	before := tempPassDirs()

	const target = 2.0
	p := video(preset.Video{Codec: "h264", RateMode: "size", TargetSizeMB: target, Speed: "fast"}, preset.Audio{Mode: "aac", Bitrate: "128k"})
	var sawPass2 bool
	out, err := tl.Run(context.Background(), p, src, func(pr runner.Progress) {
		if pr.Passes == 2 && pr.Pass == 2 {
			sawPass2 = true
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	got := fileMB(t, out)
	t.Logf("two-pass CPU: target %.1f MB, got %.2f MB (%.1f%%)", target, got, got/target*100)
	if got > target*1.02 || got < target*0.80 {
		t.Errorf("size %.2f MB is not close to %.1f MB", got, target)
	}
	if !sawPass2 {
		t.Error("never reported pass 2/2")
	}
	if tempPassDirs() != before {
		t.Error("pass log directory was not removed")
	}
}

func TestSizeModeNoAudioAndCopyAudio(t *testing.T) {
	tl := setup(t)
	needEncoder(t, "libx264")
	dir := t.TempDir()
	silent := genVideo(t, dir, "silent.mp4", 20, false)
	withAudio := genVideo(t, dir, "audio.mp4", 20, true)

	for name, c := range map[string]struct {
		src string
		a   preset.Audio
	}{
		"silent": {silent, preset.Audio{Mode: "copy"}},
		"copy":   {withAudio, preset.Audio{Mode: "copy"}},
	} {
		p := video(preset.Video{Codec: "h264", RateMode: "size", TargetSizeMB: 1.5, Speed: "fast"}, c.a)
		out, err := tl.Run(context.Background(), p, c.src, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := fileMB(t, out)
		t.Logf("%s: target 1.5 MB, got %.2f MB", name, got)
		if got > 1.5*1.03 || got < 1.5*0.75 {
			t.Errorf("%s: %.2f MB is not close to 1.5 MB", name, got)
		}
	}
}

func TestSizeModeNVENC(t *testing.T) {
	tl := setup(t)
	for _, c := range []struct{ codec, enc string }{{"h264", "h264_nvenc"}, {"h265", "hevc_nvenc"}, {"av1", "av1_nvenc"}} {
		t.Run(c.codec, func(t *testing.T) {
			needEncoder(t, c.enc)
			src := genVideo(t, t.TempDir(), "src.mp4", 20, true)
			p := video(preset.Video{Codec: c.codec, Encoder: "nvenc", RateMode: "size", TargetSizeMB: 2, Speed: "medium"}, preset.Audio{Mode: "copy"})
			out, err := tl.Run(context.Background(), p, src, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := fileMB(t, out)
			t.Logf("NVENC %s size mode: target 2.0 MB, got %.2f MB", c.codec, got)
			if got > 2*1.10 || got < 2*0.5 {
				t.Errorf("%.2f MB is not near 2 MB (hardware VBR does not guarantee the size)", got)
			}
		})
	}
}

func TestTwoPassCleanupOnCancel(t *testing.T) {
	tl := setup(t)
	needEncoder(t, "libx264")
	src := genVideo(t, t.TempDir(), "src.mp4", 20, true)
	before := tempPassDirs()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(700 * time.Millisecond); cancel() }()
	p := video(preset.Video{Codec: "h264", RateMode: "size", TargetSizeMB: 3, Speed: "slowest", ScaleMode: "stretch", Width: 3840, Height: 2160}, preset.Audio{Mode: "copy"})
	_, err := tl.Run(ctx, p, src, nil)
	if err != runner.ErrCanceled {
		t.Fatalf("want canceled, got %v", err)
	}
	if tempPassDirs() != before {
		t.Error("pass logs left behind after cancel")
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(src), "*.e2e.mp4")); len(m) != 0 {
		t.Errorf("partial output left behind: %v", m)
	}
}

// ffprobe cannot read the first file: that item fails with a clear message, the queue carries on.
func TestQueueContinuesAfterProbeFailure(t *testing.T) {
	tl := setup(t)
	needEncoder(t, "libx264")
	dir := t.TempDir()
	bad := filepath.Join(dir, "not-a-video.mp4")
	if err := os.WriteFile(bad, []byte("this is not media"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := genVideo(t, dir, "good.mp4", 5, true)

	done := make(chan struct{}, 8)
	q := runner.NewQueue(tl, func() {
		select {
		case done <- struct{}{}:
		default:
		}
	})
	p := video(preset.Video{Codec: "h264", RateMode: "size", TargetSizeMB: 1, Speed: "fast"}, preset.Audio{Mode: "copy"})
	q.Add(p, bad, good)

	deadline := time.After(60 * time.Second)
	for {
		items := q.Snapshot()
		if len(items) == 2 && items[0].Status == runner.Failed && items[1].Status == runner.Done {
			t.Logf("bad file error: %v", items[0].Err)
			if items[0].Err == nil || strings.Contains(items[0].Err.Error(), "exit status") && !strings.Contains(items[0].Err.Error(), "ffprobe") {
				t.Errorf("error must be a friendly ffprobe message: %v", items[0].Err)
			}
			return
		}
		select {
		case <-done:
		case <-deadline:
			t.Fatalf("queue did not finish: %+v", items)
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func TestSizeModeTooSmallTargetFailsClearly(t *testing.T) {
	tl := setup(t)
	needEncoder(t, "libx264")
	src := genVideo(t, t.TempDir(), "src.mp4", 20, true)
	p := video(preset.Video{Codec: "h264", RateMode: "size", TargetSizeMB: 0.01}, preset.Audio{Mode: "copy"})
	_, err := tl.Run(context.Background(), p, src, nil)
	if err == nil || !strings.Contains(err.Error(), "слишком мал") {
		t.Fatalf("want a 'too small' error, got %v", err)
	}
}
