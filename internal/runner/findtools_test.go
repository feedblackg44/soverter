package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindToolsAcceptsFolder(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(bin, "ffmpeg.exe")
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, bin} {
		tl, err := FindTools(dir)
		if err != nil || tl.FFmpeg != exe {
			t.Fatalf("FindTools(%q) = %q, %v; want %q", dir, tl.FFmpeg, err, exe)
		}
	}
	if _, err := FindTools(t.TempDir()); err == nil {
		t.Fatal("an empty folder must not be accepted")
	}
}
