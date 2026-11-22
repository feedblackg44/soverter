package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"vidpreset/internal/preset"
)

// Picker shows a list of presets; choosing one calls onPick and closes the window.
// onCancel runs if the window is closed without a choice.
func Picker(a fyne.App, presets []preset.Preset, onPick func(preset.Preset), onCancel func()) {
	w := a.NewWindow("Выберите пресет")
	picked := false
	l := widget.NewList(
		func() int { return len(presets) },
		func() fyne.CanvasObject { return widget.NewLabel("preset") },
		func(i widget.ListItemID, o fyne.CanvasObject) { o.(*widget.Label).SetText(presets[i].Name) },
	)
	l.OnSelected = func(i widget.ListItemID) {
		picked = true
		w.Close()
		onPick(presets[i])
	}
	w.SetContent(l)
	w.Resize(fyne.NewSize(360, 320))
	w.SetOnClosed(func() {
		if !picked && onCancel != nil {
			onCancel()
		}
	})
	w.Show()
}

func layoutSpacer() fyne.CanvasObject { return layout.NewSpacer() }
