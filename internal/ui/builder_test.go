package ui

import (
	"testing"

	"vidpreset/internal/preset"
)

func TestSizeLabelFor(t *testing.T) {
	cases := []struct {
		v    preset.Video
		want string
	}{
		{preset.Video{ScaleMode: preset.ScaleNone}, sizeNone},
		{preset.Video{ScaleMode: preset.ScaleStretch, Width: 1920, Height: 1080}, "1080p (1920×1080)"},
		{preset.Video{ScaleMode: preset.ScaleFit, Width: 2560, Height: 1440}, "1440p (2560×1440)"},
		{preset.Video{ScaleMode: preset.ScaleKeep, Height: 720}, "720p (1280×720)"},
		{preset.Video{ScaleMode: preset.ScaleStretch, Width: 1000, Height: 1000}, sizeCustom},
		{preset.Video{ScaleMode: preset.ScaleKeep, Height: 800}, sizeCustom},
	}
	for _, c := range cases {
		if got := sizeLabelFor(c.v); got != c.want {
			t.Errorf("%+v: got %q want %q", c.v, got, c.want)
		}
	}
}

func TestOptionMapping(t *testing.T) {
	for _, set := range [][]opt{codecOpts, familyNames, rateOpts, speedOpts, scaleModeOpts, audioOpts} {
		for _, o := range set {
			if valOf(set, labelOf(set, o.val)) != o.val {
				t.Errorf("round trip failed for %q", o.val)
			}
		}
	}
	if labelOf(codecOpts, "nope") != "" || valOf(codecOpts, "nope") != "" {
		t.Error("unknown values map to empty")
	}
}

func TestSlugAndNumbers(t *testing.T) {
	if Slug("4:3 → 16:9 (FHD)") != "4-3-16-9-fhd" || Slug("Громкость") != "preset" {
		t.Errorf("Slug: %q %q", Slug("4:3 → 16:9 (FHD)"), Slug("Громкость"))
	}
	if parseNum(" 2,5 ") != 2.5 || parseNum("abc") != 0 || fmtNum(2.50) != "2.5" {
		t.Error("number parsing")
	}
}
