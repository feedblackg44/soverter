package gui

import (
	"testing"

	"soverter/internal/preset"
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

func TestStepScale(t *testing.T) {
	defer SetScale(100)
	SetScale(100)
	if got := stepScale(1); got != 110 {
		t.Fatalf("100 -> %d", got)
	}
	SetScale(250)
	if got := stepScale(1); got != 250 {
		t.Fatalf("must clamp at the top, got %d", got)
	}
	SetScale(75)
	if got := stepScale(-1); got != 75 {
		t.Fatalf("must clamp at the bottom, got %d", got)
	}
	SetScale(120) // off-grid value goes to the neighbours
	if up, down := stepScale(1), stepScale(-1); up != 125 || down != 110 {
		t.Fatalf("off-grid: up %d down %d", up, down)
	}
}

func TestCustomSizeStaysCustom(t *testing.T) {
	e := demo(t)
	e.pickSize("1080")
	e.pickSize(sizeCustom) // values still equal 1080p, but the user asked for "custom"
	if !e.sizeCustom {
		t.Fatal("custom resolution must be remembered even when the values match a preset")
	}
	e.pickSize("720")
	if e.sizeCustom {
		t.Fatal("picking a standard size clears the custom flag")
	}
}

func TestFinishDragReorders(t *testing.T) {
	e := demo(t)
	first := e.cfg.Presets[0].ID
	third := e.cfg.Presets[2].ID
	e.rowPitch = 100
	e.dragIdx, e.dragDY = 0, 210 // dragged two rows down
	if got := e.dragTarget(); got != 2 {
		t.Fatalf("target %d", got)
	}
	e.finishDrag()
	if e.cfg.Presets[2].ID != first || e.cfg.Presets[0].ID == first || e.sel != 2 {
		t.Fatalf("order after drag wrong: %v sel %d", ids(e), e.sel)
	}
	if e.cfg.Presets[1].ID != third {
		t.Fatalf("the rows in between must shift up: %v", ids(e))
	}
	if !e.dirty {
		t.Fatal("reordering must mark the config dirty")
	}
	e.dragIdx, e.dragDY = 2, -1000 // far above the list: clamp to the first slot
	e.finishDrag()
	if e.cfg.Presets[0].ID != first {
		t.Fatalf("clamp failed: %v", ids(e))
	}
}

func ids(e *Editor) []string {
	var s []string
	for _, p := range e.cfg.Presets {
		s = append(s, p.ID)
	}
	return s
}
