package gui

import (
	"strings"
	"testing"
	"time"

	"soverter/internal/i18n"
	"soverter/internal/mediainfo"
)

func TestInfoSections(t *testing.T) {
	i18n.SetLang("en")
	defer i18n.SetLang("")
	in := mediainfo.Info{
		Path: `C:\v\clip.mkv`, Container: "Matroska", Size: 400 << 20, Duration: 125 * time.Second, BitRate: 26_000_000,
		Video: []mediainfo.Video{{Codec: "hevc", Profile: "Main 10", Width: 3840, Height: 2160, Aspect: "16:9", FPS: 59.94, PixFmt: "yuv420p10le", BitDepth: 10, HDR: "HDR10"}},
		Audio: []mediainfo.Audio{{Codec: "aac", Channels: 6, Layout: "5.1", SampleRate: 48000, BitRate: 384_000, Language: "eng"}},
	}
	text := infoText(in)
	for _, want := range []string{
		"clip.mkv", "Container: Matroska", "Size: 400.0 MB", "Duration: 2:05", "Overall bitrate: 26.00 Mbit/s",
		"Codec: hevc (Main 10)", "Resolution: 3840×2160 · 16:9", "Frame rate: 59.94 fps", "HDR: HDR10", "yuv420p10le, 10-bit",
		"Channels: 5.1", "Sample rate: 48 kHz", "Bitrate: 384 kbit/s", "Language: eng",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// no per-stream video bitrate: estimated as overall minus audio
	if !strings.Contains(text, "Bitrate: ≈ 25.62 Mbit/s (estimate)") {
		t.Errorf("video bitrate estimate missing:\n%s", text)
	}
}

func TestInfoSectionsSkipUnknown(t *testing.T) {
	i18n.SetLang("en")
	defer i18n.SetLang("")
	secs := infoSections(mediainfo.Info{Path: "x.mp4", Video: []mediainfo.Video{{Codec: "h264"}}})
	for _, s := range secs {
		for _, r := range s.Rows {
			if r[1] == "" {
				t.Fatalf("empty value shown for %q", r[0])
			}
		}
	}
}

func TestFormatHelpers(t *testing.T) {
	i18n.SetLang("en")
	defer i18n.SetLang("")
	if fmtSize(1536<<20) != "1.50 GB" || fmtSize(2048) != "2 KB" || fmtSize(0) != "" {
		t.Fatalf("sizes: %q %q %q", fmtSize(1536<<20), fmtSize(2048), fmtSize(0))
	}
	if fmtDuration(3725*time.Second) != "1:02:05" || fmtDuration(59*time.Second) != "0:59" {
		t.Fatalf("durations: %q %q", fmtDuration(3725*time.Second), fmtDuration(59*time.Second))
	}
	if trimFloat(30) != "30" || trimFloat(29.97) != "29.97" {
		t.Fatalf("trimFloat: %q %q", trimFloat(30), trimFloat(29.97))
	}
}
