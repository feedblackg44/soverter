package gui

import (
	"testing"

	"vidpreset/internal/preset"
)

func demo(t *testing.T) *Editor {
	t.Helper()
	return newEditor("", preset.Defaults(), EditorHooks{})
}

func TestVolumeForcesAudioReencode(t *testing.T) {
	e := demo(t)
	e.editVideo(func(_ *preset.Video, a *preset.Audio) { a.Mode, a.VolumeDB = preset.AudioCopy, -6 })
	_, a := e.cur().Normalized()
	if a.Mode != preset.AudioAAC || a.Bitrate == "" {
		t.Fatalf("copy audio with volume must become AAC with a bitrate, got %+v", a)
	}
}

func TestCopyCodecDropsScaleAndFPS(t *testing.T) {
	e := demo(t) // preset 0: 4:3 -> FHD stretch
	e.editVideo(func(v *preset.Video, _ *preset.Audio) { v.Codec = preset.CodecCopy })
	v, _ := e.cur().Normalized()
	if v.ScaleMode != preset.ScaleNone || v.Width != 0 || v.FPS != 0 {
		t.Fatalf("copy must reset scaling, got %+v", v)
	}
	if len(e.issues) != 0 {
		t.Fatalf("unexpected issues after switching to copy: %+v", e.issues)
	}
}

func TestSizeChoices(t *testing.T) {
	e := demo(t)
	e.pickSize("720")
	v, _ := e.cur().Normalized()
	if v.Height != 720 || v.Width != 1280 || sizeKeyFor(v) != "720" {
		t.Fatalf("720p: %+v", v)
	}
	e.pickSize(sizeNone)
	v, _ = e.cur().Normalized()
	if v.ScaleMode != preset.ScaleNone || v.Width != 0 {
		t.Fatalf("none: %+v", v)
	}
}

func TestDirtyTracking(t *testing.T) {
	e := demo(t)
	if e.dirty {
		t.Fatal("fresh editor must be clean")
	}
	e.cfg.Presets[0].Name = "x"
	e.recomputeDirty()
	if !e.dirty {
		t.Fatal("rename must make editor dirty")
	}
}
