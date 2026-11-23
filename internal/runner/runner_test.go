package runner

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"soverter/internal/preset"
)

func TestBuildArgs(t *testing.T) {
	p := preset.Preset{VF: "scale=1:1", AF: "volume=-6dB", Args: []string{"-c:a", "copy"}}
	got := BuildArgs(p, "in a.mp4", "out.mp4")
	want := []string{"-hide_banner", "-n", "-i", "in a.mp4", "-vf", "scale=1:1", "-af", "volume=-6dB", "-c:a", "copy", "out.mp4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	got = BuildArgs(preset.Preset{Args: []string{"-c:v", "libx265"}}, "i", "o")
	want = []string{"-hide_banner", "-n", "-i", "i", "-c:v", "libx265", "o"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
}

func TestOutputPath(t *testing.T) {
	p := preset.Preset{Suffix: "fhd", Ext: "mp4"}
	d := filepath.FromSlash("/v")
	j := func(s string) string { return filepath.Join(d, s) }
	cases := []struct{ in, want string }{
		{j("clip.avi"), j("clip.fhd.mp4")},
		{j("мой фильм.mkv"), j("мой фильм.fhd.mp4")},
		{j("a.b.c.mov"), j("a.b.c.fhd.mp4")},
	}
	for _, c := range cases {
		if got := OutputPath(c.in, p, func(string) bool { return false }); got != c.want {
			t.Errorf("got %s want %s", got, c.want)
		}
	}
	taken := map[string]bool{j("x.fhd.mp4"): true, j("x.fhd-1.mp4"): true}
	if got := OutputPath(j("x.mkv"), p, func(s string) bool { return taken[s] }); got != j("x.fhd-2.mp4") {
		t.Errorf("collision: %s", got)
	}
	same := preset.Preset{Suffix: "vol", Ext: "same"}
	if got := OutputPath(j("a.mkv"), same, func(string) bool { return false }); got != j("a.vol.mkv") {
		t.Errorf("same: %s", got)
	}
}

func TestParseProgress(t *testing.T) {
	var p Progress
	lines := []string{"frame=10", "out_time_us=2500000", "speed=1.5x", "progress=continue"}
	ended := 0
	for _, l := range lines {
		if ParseProgressLine(l, &p) {
			ended++
		}
	}
	if ended != 1 || p.OutTime != 2500*time.Millisecond || p.Speed != 1.5 || p.Done {
		t.Fatalf("%+v", p)
	}
	ParseProgressLine("speed=N/A", &p)
	ParseProgressLine("progress=end", &p)
	if !p.Done {
		t.Fatal("end not detected")
	}
	eta, ok := ETA(10*time.Second, Progress{OutTime: 5 * time.Second, Speed: 2.5})
	if !ok || eta != 2*time.Second {
		t.Fatalf("eta %v %v", eta, ok)
	}
	if _, ok := ETA(0, p); ok {
		t.Fatal("unknown duration must give no eta")
	}
}

func TestEncoders(t *testing.T) {
	if got := RequiredEncoders([]string{"-c:v", "libx264", "-c:a", "copy", "-c:a", "aac"}); !reflect.DeepEqual(got, []string{"libx264", "aac"}) {
		t.Fatalf("%q", got)
	}
	list := " V....D libx264              H.264\n A....D aac                  AAC\n"
	if !encoderListed(list, "libx264") || encoderListed(list, "libx265") {
		t.Fatal("encoderListed")
	}
}
