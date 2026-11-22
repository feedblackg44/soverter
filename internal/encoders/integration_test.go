package encoders

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// Runs the real trial encodes on this machine; informational (hardware differs per machine).
func TestProbeReal(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	start := time.Now()
	r, err := Probe(context.Background(), ff, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("probe took %v", time.Since(start).Round(time.Millisecond))
	for _, n := range append(append([]string{}, Software...), Hardware...) {
		s := r[n]
		t.Logf("%-12s ok=%-5v %s %s", n, s.Available, s.Reason, s.Detail)
	}
	if !r["libx264"].Available && ListContains("x", "x") {
		t.Log("libx264 not available in this build")
	}
	// second call must come from the disk cache and be much faster
	start = time.Now()
	r2, err := Probe(context.Background(), ff, false)
	if err != nil || len(r2) != len(r) {
		t.Fatalf("cached probe: %v", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("cached probe too slow: %v", d)
	}
}
