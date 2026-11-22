package e2e

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vidpreset/internal/preset"
)

func probeStream(t *testing.T, path string) (codec, size string) {
	t.Helper()
	out, err := exec.Command(tools.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries",
		"stream=codec_name,width,height", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	f := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(f) < 3 {
		t.Fatalf("unexpected ffprobe output %q", out)
	}
	return f[0], f[1] + "x" + f[2]
}

// Acceptance: «4:3 -> 16:9 FHD» built from the form gives name.fhd.mp4, 1920x1080, source untouched.
func TestStretchFHDFromForm(t *testing.T) {
	tl := setup(t)
	needEncoder(t, "libx264")
	dir := t.TempDir()
	src := genVideo(t, dir, "кино 4x3.mp4", 5, true) // 640x480 = 4:3, with a space and Cyrillic
	before, _ := os.ReadFile(src)

	var p preset.Preset
	for _, d := range preset.Defaults().Presets {
		if d.ID == "stretch-fhd" {
			p = d
		}
	}
	out, err := tl.Run(context.Background(), p, src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(out) != "кино 4x3.fhd.mp4" {
		t.Fatalf("name: %s", out)
	}
	codec, size := probeStream(t, out)
	if codec != "h264" || size != "1920x1080" {
		t.Fatalf("got %s %s", codec, size)
	}
	if after, _ := os.ReadFile(src); !bytes.Equal(before, after) {
		t.Fatal("source modified")
	}
}

// A legacy preset (raw vf/args only) still runs exactly as before.
func TestLegacyPresetStillRuns(t *testing.T) {
	tl := setup(t)
	needEncoder(t, "libx264")
	src := genVideo(t, t.TempDir(), "old.mp4", 3, true)
	p := preset.Preset{ID: "old", Name: "old", Suffix: "old", Ext: "mp4", VF: "scale=1280:720,setsar=1",
		Args: []string{"-c:v", "libx264", "-crf", "23", "-c:a", "copy"}}
	out, err := tl.Run(context.Background(), p, src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, size := probeStream(t, out); size != "1280x720" {
		t.Fatalf("size %s", size)
	}
}

// Every encoder family available on this machine x every rate mode produces a valid file.
func TestRateModeMatrix(t *testing.T) {
	tl := setup(t)
	src := genVideo(t, t.TempDir(), "src.mp4", 20, true)

	type enc struct{ name, codec, family, ffcodec string }
	encs := []enc{
		{"libx264", "h264", "cpu", "h264"},
		{"libx265", "h265", "cpu", "hevc"},
		{"h264_nvenc", "h264", "nvenc", "h264"},
		{"hevc_nvenc", "h265", "nvenc", "hevc"},
		{"av1_nvenc", "av1", "nvenc", "av1"},
	}
	for _, e := range encs {
		for _, mode := range []string{"quality", "bitrate", "size"} {
			t.Run(e.name+"/"+mode, func(t *testing.T) {
				needEncoder(t, e.name)
				v := preset.Video{Codec: e.codec, Encoder: e.family, RateMode: mode, Speed: "fast",
					ScaleMode: "stretch", Width: 1280, Height: 720}
				switch mode {
				case "quality":
					v.Quality = 26
				case "bitrate":
					v.VideoBitrate = 1.5
				case "size":
					v.TargetSizeMB = 2
				}
				p := video(v, preset.Audio{Mode: "aac", Bitrate: "128k"})
				p.Suffix = strings.ReplaceAll(e.name+"-"+mode, "_", "")
				out, err := tl.Run(context.Background(), p, src, nil)
				if err != nil {
					t.Fatal(err)
				}
				codec, size := probeStream(t, out)
				if codec != e.ffcodec || size != "1280x720" {
					t.Fatalf("got %s %s, want %s 1280x720", codec, size, e.ffcodec)
				}
				mb := fileMB(t, out)
				t.Logf("%-12s %-8s -> %-5s %s  %.2f MB", e.name, mode, codec, size, mb)
				if mode == "size" && (mb > 2*1.10 || mb < 2*0.5) {
					t.Errorf("size mode: %.2f MB is far from 2 MB", mb)
				}
			})
		}
	}
}

// Volume forces audio re-encoding; fit mode adds black bars to the exact frame.
func TestFitAndVolume(t *testing.T) {
	tl := setup(t)
	needEncoder(t, "libx264")
	src := genVideo(t, t.TempDir(), "src.mp4", 3, true)
	p := video(preset.Video{Codec: "h264", Quality: 28, Speed: "fast", ScaleMode: "fit", Width: 1920, Height: 1080},
		preset.Audio{Mode: "aac", Bitrate: "128k", VolumeDB: -6})
	out, err := tl.Run(context.Background(), p, src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, size := probeStream(t, out); size != "1920x1080" {
		t.Fatalf("fit must give exactly 1920x1080, got %s", size)
	}
}
