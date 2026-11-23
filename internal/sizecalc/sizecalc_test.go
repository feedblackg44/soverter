package sizecalc

import (
	"math"
	"strings"
	"testing"
	"time"

	"soverter/internal/preset"
)

const (
	withAudio = `{"streams":[{"codec_type":"video","bit_rate":"4000000"},{"codec_type":"audio","bit_rate":"160000"}],
	  "format":{"duration":"10.000000","bit_rate":"4200000"}}`
	noAudio  = `{"streams":[{"codec_type":"video","bit_rate":"4000000"}],"format":{"duration":"10.0","bit_rate":"4000000"}}`
	mkvNoBps = `{"streams":[{"codec_type":"video"},{"codec_type":"audio"}],"format":{"duration":"60.0"}}`
	derived  = `{"streams":[{"codec_type":"video","bit_rate":"3000000"},{"codec_type":"audio"}],"format":{"duration":"60.0","bit_rate":"3256000"}}`
	noDur    = `{"streams":[{"codec_type":"video"}],"format":{"duration":"N/A"}}`
)

func TestParseProbe(t *testing.T) {
	m, err := ParseProbe([]byte(withAudio))
	if err != nil || m.Duration != 10*time.Second || !m.HasAudio || m.AudioKbps != 160 || m.AudioEstimate {
		t.Fatalf("%+v %v", m, err)
	}
	m, _ = ParseProbe([]byte(noAudio))
	if m.HasAudio || m.AudioKbps != 0 {
		t.Fatalf("no audio: %+v", m)
	}
	m, _ = ParseProbe([]byte(mkvNoBps))
	if !m.HasAudio || m.AudioKbps != FallbackAudioKbps || !m.AudioEstimate {
		t.Fatalf("assumed audio bitrate: %+v", m)
	}
	m, _ = ParseProbe([]byte(derived))
	if m.AudioKbps != 256 || !m.AudioEstimate {
		t.Fatalf("derived from container: %+v", m)
	}
	m, _ = ParseProbe([]byte(noDur))
	if m.Duration != 0 {
		t.Fatalf("N/A duration must be zero: %+v", m)
	}
	if _, err := ParseProbe([]byte("not json")); err == nil {
		t.Fatal("garbage must fail")
	}
}

func near(t *testing.T, got int, want float64) {
	t.Helper()
	if math.Abs(float64(got)-want) > 1 {
		t.Fatalf("got %d kbps, want ~%.1f", got, want)
	}
}

// total = size * 8192 * 0.95 / duration ; video = total - audio
func TestVideoKbpsShortVideoAAC(t *testing.T) {
	m := Media{Duration: 10 * time.Second, HasAudio: true, AudioKbps: 160}
	k, err := VideoKbps(5, m, preset.Audio{Mode: "aac", Bitrate: "128k"})
	if err != nil {
		t.Fatal(err)
	}
	near(t, k, 5*8192*0.95/10-128) // 3763.2
}

func TestVideoKbpsNoAudioStream(t *testing.T) {
	m := Media{Duration: 10 * time.Second} // video without sound
	for _, mode := range []string{"copy", "aac", "remove"} {
		k, err := VideoKbps(5, m, preset.Audio{Mode: mode, Bitrate: "192k"})
		if err != nil {
			t.Fatal(err)
		}
		near(t, k, 5*8192*0.95/10) // 3891.2: no audio is subtracted whatever the mode
	}
}

func TestVideoKbpsCopyAudioUsesRealBitrate(t *testing.T) {
	m := Media{Duration: 10 * time.Second, HasAudio: true, AudioKbps: 160}
	k, _ := VideoKbps(5, m, preset.Audio{Mode: "copy"})
	near(t, k, 5*8192*0.95/10-160) // 3731.2
}

func TestVideoKbpsRemoveAudioIgnoresSourceAudio(t *testing.T) {
	m := Media{Duration: 10 * time.Second, HasAudio: true, AudioKbps: 160}
	k, _ := VideoKbps(5, m, preset.Audio{Mode: "remove"})
	near(t, k, 5*8192*0.95/10)
}

func TestVideoKbps50MBMovie(t *testing.T) {
	m := Media{Duration: 3 * time.Minute, HasAudio: true, AudioKbps: 128}
	k, err := VideoKbps(50, m, preset.Audio{Mode: "copy"})
	if err != nil {
		t.Fatal(err)
	}
	near(t, k, 50*8192*0.95/180-128)
	// the resulting size really lands under the target: (video+audio)*sec/8192 <= 50 MB
	if mb := (float64(k) + 128) * 180 / 8192; mb > 50 {
		t.Fatalf("estimated %.2f MB exceeds the target", mb)
	}
}

func TestVideoKbpsErrors(t *testing.T) {
	if _, err := VideoKbps(50, Media{}, preset.Audio{Mode: "copy"}); err != ErrNoDuration {
		t.Fatalf("unknown duration: %v", err)
	}
	if _, err := VideoKbps(0, Media{Duration: time.Second}, preset.Audio{Mode: "copy"}); err == nil {
		t.Fatal("zero target")
	}
	_, err := VideoKbps(1, Media{Duration: time.Hour, HasAudio: true, AudioKbps: 128}, preset.Audio{Mode: "copy"})
	if err == nil || !strings.Contains(err.Error(), "слишком мал") {
		t.Fatalf("too small target must explain: %v", err)
	}
}

func TestParseBitrate(t *testing.T) {
	cases := map[string]float64{"192k": 192, "1.5M": 1500, "128000": 128, " 96K ": 96}
	for in, want := range cases {
		got, err := ParseBitrate(in)
		if err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "abc", "-5k", "0"} {
		if _, err := ParseBitrate(bad); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
}

func TestOutputAudioKbpsDefaults(t *testing.T) {
	m := Media{HasAudio: true, AudioKbps: 300}
	if OutputAudioKbps(m, preset.Audio{Mode: "aac"}) != 192 || OutputAudioKbps(m, preset.Audio{Mode: "opus"}) != 128 {
		t.Fatal("default encoder bitrates")
	}
}
