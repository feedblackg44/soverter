package gui

import (
	"soverter/internal/mediainfo"
	"soverter/internal/preset"
)

// DemoEditor builds an editor without a window (for headless rendering and tests).
func DemoEditor(cfg *preset.Config, sel int) View {
	e := newEditor("", cfg, EditorHooks{})
	e.sel = sel
	return e
}

// DemoInfo builds the video properties view with already loaded data (for headless rendering).
func DemoInfo(files ...mediainfo.Info) View {
	v := &InfoView{}
	v.list.Axis = 1
	for i := range files {
		in := files[i]
		v.files = append(v.files, &infoFile{path: in.Path, info: &in})
	}
	return v
}
