package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vidpreset/internal/preset"
)

func TestRunReal(t *testing.T) {
	tools, err := FindTools("")
	if err != nil || tools.FFprobe == "" {
		t.Skip("ffmpeg/ffprobe not available")
	}
	if !tools.HasEncoder("libx264") {
		t.Skip("no libx264")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "тест файл.mp4")
	gen := exec.Command(tools.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=640x480:rate=25:duration=2",
		"-pix_fmt", "yuv420p", "-c:v", "libx264", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("gen: %v %s", err, out)
	}
	before, _ := os.ReadFile(src)

	p := preset.Defaults().Presets[0] // stretch-fhd
	var last Progress
	out, err := tools.Run(context.Background(), p, src, func(pr Progress) { last = pr })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(out) != "тест файл.fhd.mp4" {
		t.Fatalf("name %s", out)
	}
	if !last.Done {
		t.Error("no final progress")
	}
	res, _ := exec.Command(tools.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries",
		"stream=width,height", "-of", "csv=p=0", out).Output()
	if strings.TrimSpace(string(res)) != "1920,1080" {
		t.Fatalf("size %q", res)
	}
	after, _ := os.ReadFile(src)
	if string(before) != string(after) {
		t.Fatal("source modified")
	}
	// second run must not overwrite
	out2, err := tools.Run(context.Background(), p, src, nil)
	if err != nil || filepath.Base(out2) != "тест файл.fhd-1.mp4" {
		t.Fatalf("collision: %s %v", out2, err)
	}
	if d := tools.Duration(context.Background(), src); d < time.Second {
		t.Fatalf("duration %v", d)
	}

	// cancel removes partial file
	ctx, cancel := context.WithCancel(context.Background())
	slow := preset.Preset{ID: "s", Name: "s", Suffix: "slow", Ext: "mp4", VF: "scale=3840:2160",
		Args: []string{"-c:v", "libx264", "-preset", "veryslow"}}
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	_, err = tools.Run(ctx, slow, src, nil)
	if err != ErrCanceled {
		t.Fatalf("want canceled, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "тест файл.slow.mp4")); err == nil {
		t.Fatal("partial file left behind")
	}
}
