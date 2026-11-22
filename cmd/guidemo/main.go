// guidemo renders the redesigned windows with fixed data: `guidemo -shot out/` writes PNGs headlessly,
// without flags it opens the picker in a real window.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"vidpreset/internal/gui"
	"vidpreset/internal/gui/theme"
	"vidpreset/internal/preset"
)

func progressScene(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	paint.FillShape(gtx.Ops, th.P.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	rows := []gui.QueueRow{
		{File: "отпуск 4x3.mp4", Preset: "4:3 → FHD", Frac: 0.62, State: "running", Info: "62% · 2.40x · осталось 00:00:41"},
		{File: "лекция.mkv", Preset: "H.265", Frac: -1, State: "pending", Info: "В очереди"},
		{File: "клип.mp4", Preset: "Громкость −6 дБ", Frac: -1, State: "done", Info: "Готово · 18.4 МБ"},
	}
	return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				l := &layout.List{Axis: layout.Vertical}
				return l.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
					return rows[i].Layout(gtx, th)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var b gui.Btn
				return b.Layout(gtx, th, "Отмена", gui.Secondary)
			}),
		)
	})
}

func shot(path string, w, h int, p theme.Palette, draw func(gtx layout.Context, th *theme.Theme) layout.Dimensions) {
	win, err := headless.NewWindow(w, h)
	if err != nil {
		log.Fatal(err)
	}
	defer win.Release()
	th := theme.New(p)
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Constraints: layout.Exact(image.Pt(w, h)), Metric: unit.Metric{PxPerDp: 1.5, PxPerSp: 1.5}}
	paint.FillShape(gtx.Ops, th.P.Bg, clip.Rect{Max: image.Pt(w, h)}.Op())
	draw(gtx, th)
	if err := win.Frame(&ops); err != nil {
		log.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := win.Screenshot(img); err != nil {
		log.Fatal(err)
	}
	f, _ := os.Create(path)
	defer f.Close()
	_ = png.Encode(f, img)
}

func main() {
	out := flag.String("shot", "", "directory for PNG screenshots")
	flag.Parse()
	cfg := preset.Defaults()
	if *out != "" {
		_ = os.MkdirAll(*out, 0o755)
		pk := &gui.PickerView{Presets: cfg.Presets}
		shot(filepath.Join(*out, "picker-dark.png"), 660, 560, theme.DarkPalette(), pk.Layout)
		pk2 := &gui.PickerView{Presets: cfg.Presets}
		shot(filepath.Join(*out, "picker-light.png"), 660, 560, theme.LightPalette(), pk2.Layout)
		for i := 0; i < 3; i++ {
			cfg2 := preset.Defaults()
			if i == 1 {
				cfg2.Presets[5].Video.Encoder, cfg2.Presets[5].Video.RateMode, cfg2.Presets[5].Video.TargetSizeMB = "nvenc", "size", 50
				cfg2.Presets[5].Audio = &preset.Audio{Mode: "aac", Bitrate: "128k", VolumeDB: -6}
			}
			if i == 2 {
				cfg2.Presets[4] = preset.Preset{ID: "old", Name: "Старый пресет", Suffix: "old", Ext: "mp4", VF: "scale=1280:720", Args: []string{"-c:v", "libx264", "-crf", "23"}}
			}
			ed := gui.DemoEditor(cfg2, []int{0, 5, 4}[i])
			shot(filepath.Join(*out, fmt.Sprintf("editor%d.png", i)), 1770, 2500, theme.DarkPalette(), ed.Layout)
		}
		shot(filepath.Join(*out, "progress-dark.png"), 840, 480, theme.DarkPalette(), progressScene)
		return
	}
	go func() {
		w := new(app.Window)
		w.Option(app.Title("Выберите пресет"), app.Size(440, 380))
		th := theme.New(theme.DarkPalette())
		pk := &gui.PickerView{Presets: cfg.Presets, OnClose: func() { os.Exit(0) }, OnPick: func(p preset.Preset) { log.Println("picked", p.ID); os.Exit(0) }}
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.DestroyEvent:
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				pk.Layout(gtx, th)
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}
