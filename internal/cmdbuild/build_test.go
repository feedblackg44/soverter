package cmdbuild

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"vidpreset/internal/preset"
)

func sp(s string) []string { return strings.Fields(s) }

func vid(v preset.Video) preset.Preset {
	return preset.Preset{ID: "t", Name: "t", Suffix: "t", Ext: "mp4", Video: &v, Audio: &preset.Audio{Mode: preset.AudioCopy}}
}

func build(t *testing.T, p preset.Preset, o Opts) []string {
	t.Helper()
	a, err := Args(p, "in.mp4", "out.mp4", o)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// body returns the arguments between "-i in.mp4" and the output file.
func body(a []string) []string { return a[4 : len(a)-1] }

func eq(t *testing.T, got []string, want string) {
	t.Helper()
	if !reflect.DeepEqual(got, sp(want)) {
		t.Fatalf("\n got: %s\nwant: %s", strings.Join(got, " "), want)
	}
}

// --- quality mode, every encoder family and codec ---

func TestQualityAllFamilies(t *testing.T) {
	cases := []struct {
		codec, enc, want string
	}{
		{"h264", "cpu", "-c:v libx264 -crf 20 -preset slow -c:a copy"},
		{"h265", "cpu", "-c:v libx265 -crf 20 -preset slow -tag:v hvc1 -c:a copy"},
		{"av1", "cpu", "-c:v libsvtav1 -crf 20 -preset 5 -c:a copy"},
		{"h264", "nvenc", "-c:v h264_nvenc -rc vbr -cq 20 -b:v 0 -preset p5 -c:a copy"},
		{"h265", "nvenc", "-c:v hevc_nvenc -rc vbr -cq 20 -b:v 0 -preset p5 -tag:v hvc1 -c:a copy"},
		{"av1", "nvenc", "-c:v av1_nvenc -rc vbr -cq 20 -b:v 0 -preset p5 -c:a copy"},
		{"h264", "qsv", "-c:v h264_qsv -global_quality 20 -preset slow -c:a copy"},
		{"h265", "qsv", "-c:v hevc_qsv -global_quality 20 -preset slow -tag:v hvc1 -c:a copy"},
		{"av1", "qsv", "-c:v av1_qsv -global_quality 20 -preset slow -c:a copy"},
		{"h264", "amf", "-c:v h264_amf -rc cqp -qp_i 20 -qp_p 20 -qp_b 20 -quality quality -c:a copy"},
		{"h265", "amf", "-c:v hevc_amf -rc cqp -qp_i 20 -qp_p 20 -quality quality -tag:v hvc1 -c:a copy"},
		{"av1", "amf", "-c:v av1_amf -rc cqp -qp_i 20 -qp_p 20 -quality quality -c:a copy"},
	}
	for _, c := range cases {
		t.Run(c.codec+"/"+c.enc, func(t *testing.T) {
			p := vid(preset.Video{Codec: c.codec, Encoder: c.enc, Quality: 20, Speed: "slow"})
			eq(t, body(build(t, p, Opts{})), c.want)
		})
	}
}

// --- bitrate mode ---

func TestBitrateAllFamilies(t *testing.T) {
	cases := []struct{ enc, want string }{
		{"cpu", "-c:v libx264 -b:v 5M -preset medium -c:a copy"},
		{"nvenc", "-c:v h264_nvenc -rc vbr -b:v 5M -maxrate 7.5M -preset p4 -c:a copy"},
		{"amf", "-c:v h264_amf -rc vbr_peak -b:v 5M -maxrate 7.5M -quality balanced -c:a copy"},
		{"qsv", "-c:v h264_qsv -b:v 5M -maxrate 7.5M -preset medium -c:a copy"},
	}
	for _, c := range cases {
		t.Run(c.enc, func(t *testing.T) {
			p := vid(preset.Video{Codec: "h264", Encoder: c.enc, RateMode: "bitrate", VideoBitrate: 5, Speed: "medium"})
			eq(t, body(build(t, p, Opts{})), c.want)
		})
	}
	// fractional Mbit/s
	p := vid(preset.Video{Codec: "h264", Encoder: "nvenc", RateMode: "bitrate", VideoBitrate: 2.5})
	eq(t, body(build(t, p, Opts{})), "-c:v h264_nvenc -rc vbr -b:v 2.5M -maxrate 3.75M -c:a copy")
}

// --- size mode (bitrate computed by the caller) ---

func TestSizeModeFamilies(t *testing.T) {
	cases := []struct {
		enc, codec, want string
	}{
		{"cpu", "av1", "-c:v libsvtav1 -b:v 4000k -c:a copy"},                                         // single pass
		{"nvenc", "h264", "-c:v h264_nvenc -rc vbr -b:v 3800k -maxrate 4560k -c:a copy"},              // 0.95 margin, 1.2 maxrate
		{"amf", "h265", "-c:v hevc_amf -rc vbr_peak -b:v 3800k -maxrate 4560k -tag:v hvc1 -c:a copy"}, // order: codec args then tag
		{"qsv", "h264", "-c:v h264_qsv -b:v 3800k -maxrate 4560k -c:a copy"},
	}
	for _, c := range cases {
		t.Run(c.enc+"/"+c.codec, func(t *testing.T) {
			p := vid(preset.Video{Codec: c.codec, Encoder: c.enc, RateMode: "size", TargetSizeMB: 50})
			eq(t, body(build(t, p, Opts{VideoKbps: 4000})), c.want)
		})
	}
}

func TestSizeModeNeedsBitrate(t *testing.T) {
	p := vid(preset.Video{Codec: "h264", RateMode: "size", TargetSizeMB: 50})
	if _, err := Args(p, "i", "o", Opts{}); err == nil {
		t.Fatal("size mode without computed bitrate must fail")
	}
	a, err := Args(p, "i", "o", Opts{Preview: true})
	if err != nil || !strings.Contains(strings.Join(a, " "), "рассчитывается") {
		t.Fatalf("preview placeholder: %v %v", a, err)
	}
}

func TestTwoPass(t *testing.T) {
	p := vid(preset.Video{Codec: "h264", RateMode: "size", TargetSizeMB: 50, ScaleMode: "keep", Height: 720})
	if !NeedsTwoPass(p) || !NeedsSize(p) {
		t.Fatal("cpu h264 size mode must be two-pass")
	}
	p1, _ := Args(p, "in.mp4", "out.mp4", Opts{VideoKbps: 3000, Pass: 1, PassLog: "L"})
	eq(t, p1, "-hide_banner -n -i in.mp4 -vf scale=-2:720 -c:v libx264 -b:v 3000k -pass 1 -passlogfile L -an -f null "+os.DevNull)
	p2, _ := Args(p, "in.mp4", "out.mp4", Opts{VideoKbps: 3000, Pass: 2, PassLog: "L"})
	eq(t, p2, "-hide_banner -n -i in.mp4 -vf scale=-2:720 -c:v libx264 -b:v 3000k -pass 2 -passlogfile L -c:a copy out.mp4")

	for _, enc := range []string{"nvenc", "amf", "qsv"} {
		if NeedsTwoPass(vid(preset.Video{Codec: "h264", Encoder: enc, RateMode: "size", TargetSizeMB: 5})) {
			t.Errorf("%s must not use two-pass", enc)
		}
	}
	if NeedsTwoPass(vid(preset.Video{Codec: "av1", RateMode: "size", TargetSizeMB: 5})) {
		t.Error("svt-av1 must be single pass")
	}
	if NeedsTwoPass(vid(preset.Video{Codec: "h264", RateMode: "bitrate", VideoBitrate: 5})) {
		t.Error("bitrate mode is single pass")
	}
}

// --- scaling and fps ---

func TestScaleModes(t *testing.T) {
	cases := []struct {
		v    preset.Video
		want string
	}{
		{preset.Video{ScaleMode: "none"}, "-c:v libx264 -crf 18 -c:a copy"},
		{preset.Video{ScaleMode: "keep", Height: 720}, "-vf scale=-2:720 -c:v libx264 -crf 18 -c:a copy"},
		{preset.Video{ScaleMode: "stretch", Width: 1920, Height: 1080}, "-vf scale=1920:1080,setsar=1 -c:v libx264 -crf 18 -c:a copy"},
		{preset.Video{ScaleMode: "fit", Width: 1920, Height: 1080},
			"-vf scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2,setsar=1 -c:v libx264 -crf 18 -c:a copy"},
		{preset.Video{FPS: 30}, "-vf fps=30 -c:v libx264 -crf 18 -c:a copy"},
		{preset.Video{FPS: 29.97, ScaleMode: "keep", Height: 1080}, "-vf fps=29.97,scale=-2:1080 -c:v libx264 -crf 18 -c:a copy"},
	}
	for _, c := range cases {
		c.v.Codec, c.v.Quality = "h264", 18
		eq(t, body(build(t, vid(c.v), Opts{})), c.want)
	}
	// extra filters are appended to the generated chain
	p := vid(preset.Video{Codec: "h264", Quality: 18, ScaleMode: "keep", Height: 720})
	p.VF = "hqdn3d"
	eq(t, body(build(t, p, Opts{})), "-vf scale=-2:720,hqdn3d -c:v libx264 -crf 18 -c:a copy")
}

// --- audio ---

func TestAudio(t *testing.T) {
	cases := []struct {
		a    preset.Audio
		want string
	}{
		{preset.Audio{Mode: "copy"}, "-c:v copy -c:a copy"},
		{preset.Audio{Mode: "aac", Bitrate: "128k"}, "-c:v copy -c:a aac -b:a 128k"},
		{preset.Audio{Mode: "aac"}, "-c:v copy -c:a aac -b:a 192k"},
		{preset.Audio{Mode: "aac", Bitrate: "192k", VolumeDB: -6}, "-af volume=-6dB -c:v copy -c:a aac -b:a 192k"},
		{preset.Audio{Mode: "aac", Bitrate: "192k", VolumeDB: 3.5}, "-af volume=3.5dB -c:v copy -c:a aac -b:a 192k"},
		{preset.Audio{Mode: "opus", Bitrate: "96k"}, "-c:v copy -c:a libopus -b:a 96k"},
		{preset.Audio{Mode: "remove"}, "-c:v copy -an"},
	}
	for _, c := range cases {
		p := preset.Preset{ID: "t", Name: "t", Suffix: "t", Ext: "mp4", Video: &preset.Video{Codec: "copy"}, Audio: &c.a}
		eq(t, body(build(t, p, Opts{})), c.want)
	}
}

func TestExtrasAtEnd(t *testing.T) {
	p := vid(preset.Video{Codec: "h264", Quality: 18})
	p.Args = []string{"-movflags", "+faststart", "-map_metadata", "-1"}
	a := build(t, p, Opts{})
	eq(t, a[4:], "-c:v libx264 -crf 18 -c:a copy -movflags +faststart -map_metadata -1 out.mp4")
}

func TestHvc1OnlyInMP4Family(t *testing.T) {
	p := vid(preset.Video{Codec: "h265", Quality: 24})
	p.Ext = "mkv"
	if strings.Contains(strings.Join(build(t, p, Opts{}), " "), "hvc1") {
		t.Error("no hvc1 tag in mkv")
	}
	p.Ext = "same"
	if !strings.Contains(strings.Join(build(t, p, Opts{SrcExt: ".mov"}), " "), "hvc1") {
		t.Error("hvc1 for same+mov source")
	}
	if strings.Contains(strings.Join(build(t, p, Opts{SrcExt: ".mkv"}), " "), "hvc1") {
		t.Error("no hvc1 for same+mkv source")
	}
}

func TestSpeedPassThroughAndUnset(t *testing.T) {
	p := vid(preset.Video{Codec: "h264", Encoder: "nvenc", Quality: 20, Speed: "p6"})
	eq(t, body(build(t, p, Opts{})), "-c:v h264_nvenc -rc vbr -cq 20 -b:v 0 -preset p6 -c:a copy")
	p = vid(preset.Video{Codec: "h264", Quality: 20})
	eq(t, body(build(t, p, Opts{})), "-c:v libx264 -crf 20 -c:a copy")
}

// --- validation ---

func errs(p preset.Preset) []string {
	var r []string
	for _, i := range Validate(p) {
		if i.Severity == Error {
			r = append(r, i.Msg)
		}
	}
	return r
}

func warns(p preset.Preset) []string {
	var r []string
	for _, i := range Validate(p) {
		if i.Severity == Warning {
			r = append(r, i.Msg)
		}
	}
	return r
}

func TestValidateErrors(t *testing.T) {
	bad := map[string]preset.Preset{
		"copy+scale":   vid(preset.Video{Codec: "copy", ScaleMode: "keep", Height: 720}),
		"copy+fps":     vid(preset.Video{Codec: "copy", FPS: 30}),
		"copy+bitrate": vid(preset.Video{Codec: "copy", RateMode: "bitrate", VideoBitrate: 5}),
		"copy+size":    vid(preset.Video{Codec: "copy", RateMode: "size", TargetSizeMB: 50}),
		"no bitrate":   vid(preset.Video{Codec: "h264", RateMode: "bitrate"}),
		"no size":      vid(preset.Video{Codec: "h264", RateMode: "size"}),
		"keep no h":    vid(preset.Video{Codec: "h264", ScaleMode: "keep"}),
		"stretch no w": vid(preset.Video{Codec: "h264", ScaleMode: "stretch", Height: 720}),
	}
	vol := vid(preset.Video{Codec: "copy"})
	vol.Audio = &preset.Audio{Mode: "copy", VolumeDB: -6}
	bad["copy audio + volume"] = vol
	for n, p := range bad {
		if len(errs(p)) == 0 {
			t.Errorf("%s: expected an error", n)
		}
		if _, err := Args(p, "i", "o", Opts{}); err == nil {
			t.Errorf("%s: Args must fail", n)
		}
	}
	if len(errs(vid(preset.Video{Codec: "copy"}))) != 0 {
		t.Error("plain copy must be valid")
	}
}

func TestValidateContainerWarnings(t *testing.T) {
	p := vid(preset.Video{Codec: "h264", Quality: 20})
	p.Ext = "webm"
	if len(warns(p)) < 2 { // h264 in webm + copy audio
		t.Fatalf("webm+h264: %v", warns(p))
	}
	if len(errs(p)) != 0 {
		t.Fatal("container problems are warnings, not errors")
	}
	p.Video.Codec = "av1"
	p.Audio = &preset.Audio{Mode: "opus"}
	if len(warns(p)) != 0 {
		t.Fatalf("webm+av1+opus must be clean: %v", warns(p))
	}
	p.Audio = &preset.Audio{Mode: "aac"}
	if len(warns(p)) != 1 {
		t.Fatalf("webm+aac: %v", warns(p))
	}
	p.Ext = "mp3"
	if len(warns(p)) == 0 {
		t.Fatal("audio container warning")
	}
}

func TestEncodersList(t *testing.T) {
	p := vid(preset.Video{Codec: "h265", Encoder: "nvenc"})
	p.Audio = &preset.Audio{Mode: "aac"}
	if got := Encoders(p); !reflect.DeepEqual(got, []string{"hevc_nvenc", "aac"}) {
		t.Fatalf("%v", got)
	}
	if Encoders(vid(preset.Video{Codec: "copy"})) != nil {
		t.Fatal("copy needs no encoder")
	}
}

// --- backward compatibility and defaults ---

func TestLegacyPresetUnchanged(t *testing.T) {
	p := preset.Preset{ID: "old", Name: "old", Suffix: "o", Ext: "mp4", VF: "scale=1920:1080,setsar=1", AF: "volume=-6dB",
		Args: []string{"-c:v", "libx264", "-crf", "18", "-c:a", "copy"}}
	a, err := Args(p, "in a.mp4", "out.mp4", Opts{})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, a[:3], "-hide_banner -n -i")
	if a[3] != "in a.mp4" {
		t.Fatal("input path must stay one argument")
	}
	if got := strings.Join(a[4:], " "); got != "-vf scale=1920:1080,setsar=1 -af volume=-6dB -c:v libx264 -crf 18 -c:a copy out.mp4" {
		t.Fatalf("%s", got)
	}
	if Validate(p) != nil || NeedsSize(p) || NeedsTwoPass(p) || Encoders(p) != nil {
		t.Fatal("legacy presets have no structured semantics")
	}
}

// The default structured presets must produce exactly the commands of the original raw presets.
func TestDefaultsMatchOriginalCommands(t *testing.T) {
	want := map[string]string{
		"stretch-fhd": "-vf scale=1920:1080,setsar=1 -c:v libx264 -crf 18 -preset medium -c:a copy",
		"stretch-qhd": "-vf scale=2560:1440,setsar=1 -c:v libx264 -crf 18 -preset medium -c:a copy",
		"volume-down": "-af volume=-6dB -c:v copy -c:a aac -b:a 192k",
		"fps-30":      "-vf fps=30 -c:v libx264 -crf 18 -preset medium -c:a copy",
		"scale-720p":  "-vf scale=-2:720 -c:v libx264 -crf 20 -preset medium -c:a copy",
		"h265":        "-c:v libx265 -crf 24 -preset medium -tag:v hvc1 -c:a copy",
	}
	for _, p := range preset.Defaults().Presets {
		a, err := Args(p, "in.mp4", "out.mp4", Opts{SrcExt: ".mp4"})
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		if got := strings.Join(a[4:len(a)-1], " "); got != want[p.ID] {
			t.Errorf("%s:\n got: %s\nwant: %s", p.ID, got, want[p.ID])
		}
		if err := p.Validate(); err != nil {
			t.Errorf("%s: %v", p.ID, err)
		}
	}
}

func TestResolveExt(t *testing.T) {
	if ResolveExt(preset.Preset{Ext: "same"}, ".MKV") != "MKV" || ResolveExt(preset.Preset{Ext: ".mp4"}, "") != "mp4" {
		t.Fatal("ResolveExt")
	}
}

func TestTwoPassKeepsExtrasInBothPasses(t *testing.T) {
	p := vid(preset.Video{Codec: "h265", RateMode: "size", TargetSizeMB: 20})
	p.Args = []string{"-tune", "grain"}
	p1, _ := Args(p, "in.mp4", "out.mp4", Opts{VideoKbps: 900, Pass: 1, PassLog: "L"})
	eq(t, p1[4:], "-c:v libx265 -b:v 900k -tag:v hvc1 -pass 1 -passlogfile L -an -tune grain -f null "+os.DevNull)
	p2, _ := Args(p, "in.mp4", "out.mp4", Opts{VideoKbps: 900, Pass: 2, PassLog: "L"})
	eq(t, p2[4:], "-c:v libx265 -b:v 900k -tag:v hvc1 -pass 2 -passlogfile L -c:a copy -tune grain out.mp4")
}
