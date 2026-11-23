package preset

import (
	"reflect"
	"strings"
	"testing"
)

func TestDefaultsValidAndRoundTrip(t *testing.T) {
	c := Defaults()
	if err := ValidateAll(c.Presets); err != nil {
		t.Fatal(err)
	}
	data, err := Encode(c)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Presets, c2.Presets) {
		t.Fatal("round trip mismatch")
	}
}

func TestValidate(t *testing.T) {
	ok := Preset{ID: "a-1", Name: "n", Suffix: "s", Ext: "mp4", VF: "x"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Preset{
		"upper id":  {ID: "A", Name: "n", Suffix: "s", Ext: "mp4", VF: "x"},
		"space id":  {ID: "a b", Name: "n", Suffix: "s", Ext: "mp4", VF: "x"},
		"empty id":  {Name: "n", Suffix: "s", Ext: "mp4", VF: "x"},
		"no name":   {ID: "a", Suffix: "s", Ext: "mp4", VF: "x"},
		"no suffix": {ID: "a", Name: "n", Ext: "mp4", VF: "x"},
		"no ext":    {ID: "a", Name: "n", Suffix: "s", VF: "x"},
		"no action": {ID: "a", Name: "n", Suffix: "s", Ext: "mp4"},
	}
	for n, p := range bad {
		if p.Validate() == nil {
			t.Errorf("%s: expected error", n)
		}
	}
	if ValidateAll([]Preset{ok, ok}) == nil {
		t.Error("duplicate id must fail")
	}
}

func TestParseErrors(t *testing.T) {
	_, err := Parse([]byte("[[preset]]\nid = \"a\"\nname = \n"))
	if err == nil || !strings.Contains(err.Error(), "строке 3") {
		t.Fatalf("want line number, got %v", err)
	}
	_, err = Parse([]byte("[[preset]]\nid = \"Bad Id\"\nname=\"n\"\nsuffix=\"s\"\next=\"mp4\"\nvf=\"x\"\n"))
	if err == nil {
		t.Fatal("bad id must fail")
	}
}

func TestSplitJoinArgs(t *testing.T) {
	got, err := SplitArgs(`-c:v libx264 -metadata "title=a b" -x 'q r'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-c:v", "libx264", "-metadata", "title=a b", "-x", "q r"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	back, _ := SplitArgs(JoinArgs(want))
	if !reflect.DeepEqual(back, want) {
		t.Fatalf("join/split %q", back)
	}
	if _, err := SplitArgs(`"open`); err == nil {
		t.Fatal("unclosed quote must fail")
	}
}

const structuredTOML = `
[[preset]]
id = "stretch-fhd"
name = "4:3 -> 16:9 (FHD)"
suffix = "fhd"
ext = "mp4"
[preset.video]
codec = "h264"
encoder = "nvenc"
rate_mode = "quality"
quality = 18
speed = "medium"
scale_mode = "stretch"
width = 1920
height = 1080
[preset.audio]
mode = "aac"
bitrate = "192k"
volume_db = -6

[[preset]]
id = "legacy"
name = "legacy"
suffix = "l"
ext = "mp4"
vf = "scale=1280:720"
args = ["-c:v", "libx264"]
`

func TestStructuredAndLegacyCoexist(t *testing.T) {
	c, err := Parse([]byte(structuredTOML))
	if err != nil {
		t.Fatal(err)
	}
	s, l := c.Presets[0], c.Presets[1]
	if !s.Structured() || l.Structured() {
		t.Fatal("structured/legacy detection")
	}
	if s.Video.Encoder != "nvenc" || s.Video.Width != 1920 || s.Audio.VolumeDB != -6 || s.Audio.Bitrate != "192k" {
		t.Fatalf("%+v %+v", s.Video, s.Audio)
	}
	// round trip keeps both kinds intact, and legacy presets get no new tables
	data, err := Encode(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "[preset.video]") != 1 {
		t.Fatalf("legacy preset must not gain a [preset.video] table:\n%s", data)
	}
	c2, err := Parse(data)
	if err != nil || !reflect.DeepEqual(c.Presets, c2.Presets) {
		t.Fatalf("round trip: %v", err)
	}
}

func TestOldConfigWithoutNewFields(t *testing.T) {
	old := "[settings]\nmenu_mode = \"submenu\"\n[[preset]]\nid=\"a\"\nname=\"A\"\nsuffix=\"a\"\next=\"mp4\"\nvf=\"scale=1:1\"\n"
	c, err := Parse([]byte(old))
	if err != nil || c.Presets[0].Structured() {
		t.Fatalf("old config must load as a legacy preset: %v", err)
	}
	if c.Settings.Integration != "modern" {
		t.Fatal("missing integration defaults to modern")
	}
}

func TestStructuredValidation(t *testing.T) {
	mk := func(v Video, a Audio) Preset {
		return Preset{ID: "x", Name: "x", Suffix: "x", Ext: "mp4", Video: &v, Audio: &a}
	}
	if err := mk(Video{Codec: "h264"}, Audio{Mode: "copy"}).Validate(); err != nil {
		t.Fatal(err) // structured presets need no vf/af/args
	}
	bad := []Preset{
		mk(Video{Codec: "vp9"}, Audio{Mode: "copy"}),
		mk(Video{Codec: "h264", Encoder: "tpu"}, Audio{Mode: "copy"}),
		mk(Video{Codec: "h264", RateMode: "magic"}, Audio{Mode: "copy"}),
		mk(Video{Codec: "h264", ScaleMode: "zoom"}, Audio{Mode: "copy"}),
		mk(Video{Codec: "h264"}, Audio{Mode: "flac"}),
		mk(Video{Codec: "h264", Quality: 99}, Audio{Mode: "copy"}),
		mk(Video{Codec: "h264", FPS: -1}, Audio{Mode: "copy"}),
	}
	for i, p := range bad {
		if p.Validate() == nil {
			t.Errorf("case %d must fail", i)
		}
	}
	// empty tables mean defaults
	if err := (Preset{ID: "x", Name: "x", Suffix: "x", Ext: "mp4", Video: &Video{}}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStructuredTOMLHasNoZeroNoise(t *testing.T) {
	data, err := Encode(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, noise := range []string{"video_bitrate", "target_size_mb", "fps = 0", "volume_db = 0", "width = 0"} {
		if strings.Contains(s, noise) && noise != "fps = 0" {
			// fps-30 sets fps = 30; only zero values count as noise
			t.Errorf("zero field %q written to TOML", noise)
		}
	}
	if strings.Contains(s, "fps = 0.0") || strings.Contains(s, "fps = 0\n") {
		t.Error("fps = 0 written")
	}
}

func TestMenuNameDefaultsAndOverride(t *testing.T) {
	d := DefaultMenuTitle()
	if d != "Convert with Soverter" && d != "Обработать с помощью Soverter" {
		t.Fatalf("unexpected default %q", d)
	}
	if got := (Settings{}).MenuName(); got != d {
		t.Fatalf("empty setting must give the default, got %q", got)
	}
	if got := (Settings{MenuTitle: "  Edit with X "}).MenuName(); got != "Edit with X" {
		t.Fatalf("override %q", got)
	}
}
