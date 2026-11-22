package encoders

import (
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fakeList = ` Encoders:
 V....D libx264              libx264 H.264
 V....D libx265              libx265 H.265
 V....D libsvtav1            SVT-AV1
 A....D aac                  AAC
 V....D h264_nvenc           NVIDIA NVENC H.264 encoder (codec h264)
 V....D hevc_nvenc           NVIDIA NVENC hevc encoder (codec hevc)
 V....D av1_nvenc            NVIDIA NVENC av1 encoder (codec av1)
 V....D h264_amf             AMD AMF H.264 Encoder (codec h264)
 V....D h264_qsv             H.264 / AVC (Intel Quick Sync Video acceleration)
`

func TestListContains(t *testing.T) {
	if !ListContains(fakeList, "h264_nvenc") || !ListContains(fakeList, "aac") {
		t.Fatal("expected encoders to be found")
	}
	if ListContains(fakeList, "libopus") || ListContains(fakeList, "nvenc") {
		t.Fatal("false positive")
	}
}

func TestEvaluateNvidiaMachine(t *testing.T) {
	var probed int32
	probe := func(n string) error {
		atomic.AddInt32(&probed, 1)
		if strings.HasSuffix(n, "_nvenc") {
			return nil
		}
		return errors.New("Cannot load amfrt64.dll\nDevice creation failed")
	}
	r := Evaluate(fakeList, probe, 3)

	for _, n := range []string{"h264_nvenc", "hevc_nvenc", "av1_nvenc", "libx264", "libx265", "libsvtav1", "aac"} {
		if !r[n].Available {
			t.Errorf("%s must be available: %+v", n, r[n])
		}
	}
	// in the build but no hardware: probe failed
	for _, n := range []string{"h264_amf", "h264_qsv"} {
		if r[n].Available || r[n].Reason == "" {
			t.Errorf("%s must be unavailable with a reason: %+v", n, r[n])
		}
	}
	if !strings.Contains(r["h264_amf"].Reason, "AMD") || !strings.Contains(r["h264_qsv"].Reason, "Intel") {
		t.Error("reasons must name the vendor")
	}
	if r["h264_amf"].Detail != "Device creation failed" {
		t.Errorf("detail: %q", r["h264_amf"].Detail)
	}
	// not in the build at all: not even probed
	if r["hevc_amf"].Available || !strings.Contains(r["hevc_amf"].Reason, "нет этого энкодера") {
		t.Errorf("hevc_amf: %+v", r["hevc_amf"])
	}
	if r["libopus"].Available || r["libopus"].Reason == "" {
		t.Errorf("libopus missing from build: %+v", r["libopus"])
	}
	if got := atomic.LoadInt32(&probed); got != 5 { // 3 nvenc + h264_amf + h264_qsv
		t.Errorf("only encoders present in the list are probed, probed %d", got)
	}
}

func TestEvaluateRunsProbesInParallelWithLimit(t *testing.T) {
	var cur, peak int32
	probe := func(string) error {
		c := atomic.AddInt32(&cur, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if c <= p || atomic.CompareAndSwapInt32(&peak, p, c) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		return nil
	}
	Evaluate(fakeList, probe, 2)
	if peak > 2 || peak < 2 {
		t.Fatalf("peak concurrency %d, want 2", peak)
	}
}

func TestReportOKUnknownIsAvailable(t *testing.T) {
	r := Report{"h264_amf": {Reason: "x"}, "libx264": {Available: true}}
	if r.OK("h264_amf") || !r.OK("libx264") || !r.OK("some_custom_encoder") {
		t.Fatal("OK semantics")
	}
	if r.Why("h264_amf") != "x" || r.Why("libx264") != "" {
		t.Fatal("Why semantics")
	}
}

func TestDiskCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "enc.json")
	now := time.Now()
	rep := Report{"libx264": {Available: true}, "h264_amf": {Reason: "нет"}}

	if _, ok := LoadCache(path, "k", now); ok {
		t.Fatal("no cache yet")
	}
	if err := SaveCache(path, "k", rep, now); err != nil {
		t.Fatal(err)
	}
	got, ok := LoadCache(path, "k", now.Add(time.Hour))
	if !ok || !got["libx264"].Available || got["h264_amf"].Reason != "нет" {
		t.Fatalf("cache hit: %v %v", got, ok)
	}
	if _, ok := LoadCache(path, "other-ffmpeg", now); ok {
		t.Fatal("different ffmpeg build must miss")
	}
	if _, ok := LoadCache(path, "k", now.Add(CacheTTL+time.Hour)); ok {
		t.Fatal("expired cache must miss")
	}
	if _, ok := LoadCache("", "k", now); ok {
		t.Fatal("empty path")
	}
}

func TestManagerCachesInMemory(t *testing.T) {
	m := &Manager{}
	if _, ready, _ := m.Report(); ready {
		t.Fatal("not ready before the first probe")
	}
}
