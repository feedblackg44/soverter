package ffimport

import (
	"reflect"
	"strings"
	"testing"

	"soverter/internal/cmdbuild"
	"soverter/internal/preset"
)

func mustParse(t *testing.T, cmd string) Result {
	t.Helper()
	r, err := Parse(cmd)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// rebuild builds the command back from the imported preset, without the input/output parts.
func rebuild(t *testing.T, r Result) string {
	t.Helper()
	a, err := cmdbuild.Args(r.Preset, "in.mp4", "out.mp4", cmdbuild.Opts{SrcExt: ".mp4"})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	return strings.Join(a[4:len(a)-1], " ")
}

// For canonical commands the form reproduces exactly the same arguments.
func TestRoundTripCanonicalCommands(t *testing.T) {
	cases := map[string]string{
		"stretch FHD": `ffmpeg -i input.mp4 -vf "scale=1920:1080,setsar=1" -c:v libx264 -crf 18 -preset medium -c:a copy output.mp4`,
		"stretch QHD": `ffmpeg -i input.mp4 -vf scale=2560:1440,setsar=1 -c:v libx264 -crf 18 -preset medium -c:a copy output.mp4`,
		"volume":      `ffmpeg -i input.mp4 -af "volume=-6dB" -c:v copy -c:a aac -b:a 192k output.mp4`,
		"fps filter":  `ffmpeg -i input.mp4 -vf fps=30 -c:v libx264 -crf 18 -preset medium -c:a copy output.mp4`,
		"720p":        `ffmpeg -i input.mp4 -vf scale=-2:720 -c:v libx264 -crf 20 -preset medium -c:a copy output.mp4`,
		"h265":        `ffmpeg -i input.mp4 -c:v libx265 -crf 24 -preset medium -tag:v hvc1 -c:a copy output.mp4`,
		"nvenc":       `ffmpeg -i input.mp4 -c:v h264_nvenc -rc vbr -cq 19 -b:v 0 -preset p5 -c:a copy output.mp4`,
		"nvenc hevc":  `ffmpeg -i input.mp4 -c:v hevc_nvenc -rc vbr -cq 24 -b:v 0 -preset p4 -tag:v hvc1 -c:a aac -b:a 160k output.mp4`,
		"bitrate cpu": `ffmpeg -i input.mp4 -c:v libx264 -b:v 5M -preset slow -c:a aac -b:a 128k output.mp4`,
		"bitrate nv":  `ffmpeg -i input.mp4 -c:v h264_nvenc -rc vbr -b:v 5M -maxrate 7.5M -preset p4 -c:a copy output.mp4`,
		"fit":         `ffmpeg -i input.mp4 -vf "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2,setsar=1" -c:v libx264 -crf 20 -c:a copy output.mp4`,
		"av1":         `ffmpeg -i input.mp4 -c:v libsvtav1 -crf 30 -preset 5 -c:a libopus -b:a 96k output.webm`,
		"qsv":         `ffmpeg -i input.mp4 -c:v hevc_qsv -global_quality 23 -preset medium -tag:v hvc1 -c:a copy output.mp4`,
		"amf":         `ffmpeg -i input.mp4 -c:v h264_amf -rc cqp -qp_i 22 -qp_p 22 -qp_b 22 -quality balanced -c:a copy output.mp4`,
		"no audio":    `ffmpeg -i input.mp4 -c:v libx264 -crf 22 -an output.mp4`,
		"copy video":  `ffmpeg -i input.mp4 -c:v copy -c:a aac -b:a 192k output.mkv`,
		"-c copy":     `ffmpeg -i input.mp4 -c copy output.mkv`,
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			r := mustParse(t, cmd)
			if len(r.Extra) != 0 {
				t.Fatalf("nothing should be left raw: %v", r.Extra)
			}
			want := strings.Join(strings.Fields(cmd), " ")
			want = strings.TrimPrefix(want, "ffmpeg -i input.mp4 ")
			want = strings.TrimSuffix(want, " output.mp4")
			want = strings.TrimSuffix(want, " output.mkv")
			want = strings.TrimSuffix(want, " output.webm")
			want = strings.NewReplacer(`"`, "").Replace(want)
			if name == "-c copy" {
				want = "-c:v copy -c:a copy"
			}
			if got := rebuild(t, r); got != want {
				t.Fatalf("\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestFieldsFromStretchFHD(t *testing.T) {
	r := mustParse(t, `"C:\Program Files\ffmpeg\bin\ffmpeg.exe" -hide_banner -y -i "D:\видео\мой файл.mp4" -vf "scale=1920:1080,setsar=1" -c:v libx264 -crf 18 -preset medium -c:a copy "D:\видео\out.mp4"`)
	v, a := r.Preset.Video, r.Preset.Audio
	if v.Codec != "h264" || v.Encoder != "cpu" || v.Quality != 18 || v.Speed != "medium" ||
		v.ScaleMode != "stretch" || v.Width != 1920 || v.Height != 1080 || v.RateMode != "quality" {
		t.Fatalf("%+v", v)
	}
	if a.Mode != "copy" || r.Preset.Ext != "mp4" {
		t.Fatalf("%+v ext=%s", a, r.Preset.Ext)
	}
	if len(r.Extra) != 0 || len(r.Warnings) != 0 {
		t.Fatalf("extra=%v warnings=%v", r.Extra, r.Warnings)
	}
	joined := strings.Join(r.Ignored, "|")
	for _, want := range []string{"ffmpeg.exe", "-hide_banner", "-y", `мой файл.mp4`, `out.mp4`} {
		if !strings.Contains(joined, want) {
			t.Errorf("ignored list lacks %q: %v", want, r.Ignored)
		}
	}
	if len(r.Recognized) < 5 {
		t.Fatalf("recognized list too short: %v", r.Recognized)
	}
}

func TestNVENCFields(t *testing.T) {
	r := mustParse(t, `ffmpeg -hwaccel cuda -i in.mp4 -c:v hevc_nvenc -rc vbr -cq 22 -b:v 0 -preset p5 -c:a copy out.mp4`)
	v := r.Preset.Video
	if v.Codec != "h265" || v.Encoder != "nvenc" || v.Quality != 22 || v.RateMode != "quality" || v.Speed != "slow" {
		t.Fatalf("%+v", v)
	}
	// -hwaccel is an input option that the builder cannot place before -i
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "-hwaccel") {
		t.Fatalf("warnings: %v", r.Warnings)
	}
	if len(r.Extra) != 0 {
		t.Fatalf("extra: %v", r.Extra)
	}
}

func TestBitrateFields(t *testing.T) {
	r := mustParse(t, `ffmpeg -i in.mp4 -c:v libx264 -b:v 4500k -c:a aac -b:a 128k out.mp4`)
	v, a := r.Preset.Video, r.Preset.Audio
	if v.RateMode != "bitrate" || v.VideoBitrate != 4.5 || a.Mode != "aac" || a.Bitrate != "128k" {
		t.Fatalf("%+v %+v", v, a)
	}
}

func TestFPSOption(t *testing.T) {
	r := mustParse(t, `ffmpeg -i in.mp4 -r 24 -c:v libx264 -crf 20 -c:a copy out.mp4`)
	if r.Preset.Video.FPS != 24 || len(r.Extra) != 0 {
		t.Fatalf("%+v extra=%v", r.Preset.Video, r.Extra)
	}
}

func TestUnrecognizedGoesToExtra(t *testing.T) {
	r := mustParse(t, `ffmpeg -i in.mp4 -vf "hqdn3d,scale=-2:720,eq=brightness=0.06" -c:v libx264 -crf 20 -pix_fmt yuv420p -movflags +faststart -map_metadata -1 -c:a copy out.mp4`)
	v := r.Preset.Video
	if v.ScaleMode != "keep" || v.Height != 720 || v.Quality != 20 {
		t.Fatalf("%+v", v)
	}
	if r.Preset.VF != "hqdn3d,eq=brightness=0.06" {
		t.Fatalf("leftover filters: %q", r.Preset.VF)
	}
	if got := strings.Join(r.Preset.Args, " "); got != "-pix_fmt yuv420p -movflags +faststart -map_metadata -1" {
		t.Fatalf("leftover args: %q", got)
	}
	if len(r.Extra) != 2 {
		t.Fatalf("extra summary: %v", r.Extra)
	}
	// and it still builds: extra filters are appended after the generated chain
	if got := rebuild(t, r); !strings.Contains(got, "-vf scale=-2:720,hqdn3d,eq=brightness=0.06") {
		t.Fatalf("%s", got)
	}
}

func TestUnsupportedEncodersStayRaw(t *testing.T) {
	r := mustParse(t, `ffmpeg -i in.mp4 -c:v libvpx-vp9 -crf 30 -b:v 0 -c:a libmp3lame out.webm`)
	if len(r.Warnings) < 2 {
		t.Fatalf("warnings: %v", r.Warnings)
	}
	if got := strings.Join(r.Preset.Args, " "); got != "-c:v libvpx-vp9 -c:a libmp3lame" {
		t.Fatalf("args: %q", got)
	}
	if r.Preset.Ext != "webm" {
		t.Fatal("ext")
	}
}

func TestFiltersWithCommasInsideParentheses(t *testing.T) {
	got := splitFilters(`scale=iw*min(1,2):ih,fps=30,drawtext=text='a,b':x=(w-text_w)/2`)
	want := []string{`scale=iw*min(1,2):ih`, `fps=30`, `drawtext=text='a,b':x=(w-text_w)/2`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
}

func TestLineContinuations(t *testing.T) {
	ps := "ffmpeg -i in.mp4 `\n  -c:v libx264 -crf 18 `\n  -c:a copy out.mp4"
	cmd := "ffmpeg -i in.mp4 ^\r\n -c:v libx264 -crf 18 ^\r\n -c:a copy out.mp4"
	sh := "ffmpeg -i in.mp4 \\\n -c:v libx264 -crf 18 \\\n -c:a copy out.mp4"
	for _, c := range []string{ps, cmd, sh} {
		r := mustParse(t, c)
		if r.Preset.Video.Quality != 18 || r.Preset.Audio.Mode != "copy" {
			t.Fatalf("%q -> %+v", c, r.Preset.Video)
		}
	}
}

func TestWarningsForMissingCodecs(t *testing.T) {
	r := mustParse(t, `ffmpeg -i in.mp4 -vf scale=1280:720 out.mp4`)
	if len(r.Warnings) < 3 { // no -c:v, no -c:a, no setsar
		t.Fatalf("warnings: %v", r.Warnings)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Fatal("empty command")
	}
	if _, err := Parse(`ffmpeg -i "unclosed`); err == nil {
		t.Fatal("unclosed quote")
	}
}

// An imported preset must always be a valid preset.
func TestImportedPresetValidates(t *testing.T) {
	r := mustParse(t, `ffmpeg -i in.mp4 -c:v h264_nvenc -rc vbr -cq 19 -b:v 0 -preset p5 -c:a copy out.mp4`)
	p := r.Preset
	p.ID, p.Name, p.Suffix = "x", "x", "x"
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(cmdbuild.Validate(p)) != 0 {
		t.Fatalf("%v", cmdbuild.Validate(p))
	}
	_ = preset.Preset{}
}
