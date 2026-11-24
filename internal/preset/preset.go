// Package preset loads, saves and validates presets.toml.
package preset

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"soverter/internal/i18n"
	"strings"

	"github.com/BurntSushi/toml"
)

type Preset struct {
	ID     string   `toml:"id"`
	Name   string   `toml:"name"`
	Suffix string   `toml:"suffix"`
	Ext    string   `toml:"ext"`
	VF     string   `toml:"vf,omitempty"`
	AF     string   `toml:"af,omitempty"`
	Args   []string `toml:"args,omitempty"`
	// Disabled presets stay in the file and editor but are hidden from the context menu.
	Disabled bool `toml:"disabled,omitempty"`

	// Structured (form-built) presets; see structured.go. Both nil = legacy raw preset.
	Video *Video `toml:"video,omitempty"`
	Audio *Audio `toml:"audio,omitempty"`
}

// Enabled returns only the presets shown in the context menu, in order.
func Enabled(ps []Preset) []Preset {
	var out []Preset
	for _, p := range ps {
		if !p.Disabled {
			out = append(out, p)
		}
	}
	return out
}

type Settings struct {
	MenuMode    string `toml:"menu_mode"`   // "submenu" | "single"
	Integration string `toml:"integration"` // "classic" (registry verbs, default) | "modern" (Windows 11 menu, beta)
	AutoClose   bool   `toml:"auto_close"`
	FFmpegPath  string `toml:"ffmpeg_path,omitempty"`
	Language    string `toml:"language,omitempty"`          // "en" (default) | "ru"
	UIScale     int    `toml:"ui_scale,omitempty,omitzero"` // interface scale in percent (0 = 100)
	MenuTitle   string `toml:"menu_title,omitempty"`        // name of the Explorer menu item (default: the application name)
}

type Config struct {
	Settings Settings `toml:"settings"`
	Presets  []Preset `toml:"preset"`
}

var idRe = regexp.MustCompile(`^[a-z0-9-]+$`)

func ValidID(id string) bool { return idRe.MatchString(id) }

// Validate checks a single preset (not uniqueness).
func (p Preset) Validate() error {
	if !ValidID(p.ID) {
		return fmt.Errorf(i18n.T("недопустимый id %q (разрешены a-z0-9-)"), p.ID)
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf(i18n.T("preset %s: пустое name"), p.ID)
	}
	if strings.TrimSpace(p.Suffix) == "" {
		return fmt.Errorf(i18n.T("preset %s: пустой suffix"), p.ID)
	}
	if strings.TrimSpace(p.Ext) == "" {
		return fmt.Errorf(i18n.T("preset %s: пустое ext"), p.ID)
	}
	if p.Structured() {
		return p.validateStructured()
	}
	if p.VF == "" && p.AF == "" && len(p.Args) == 0 {
		return fmt.Errorf(i18n.T("preset %s: нужно хотя бы одно из vf/af/args"), p.ID)
	}
	return nil
}

// ValidateAll validates every preset and id uniqueness.
func ValidateAll(ps []Preset) error {
	seen := map[string]bool{}
	for _, p := range ps {
		if err := p.Validate(); err != nil {
			return err
		}
		if seen[p.ID] {
			return fmt.Errorf(i18n.T("повторяющийся id %q"), p.ID)
		}
		seen[p.ID] = true
	}
	return nil
}

func (c *Config) Find(id string) (Preset, bool) {
	for _, p := range c.Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Dir returns %APPDATA%\soverter (or the user config dir elsewhere).
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "soverter"), nil
}

func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "presets.toml"), nil
}

// Parse decodes and validates config data. Errors include line numbers for syntax errors.
func Parse(data []byte) (*Config, error) {
	c := &Config{Settings: Settings{MenuMode: "submenu", Integration: "classic"}}
	if _, err := toml.Decode(string(data), c); err != nil {
		if pe, ok := err.(toml.ParseError); ok {
			return nil, fmt.Errorf(i18n.T("ошибка в строке %d: %s"), pe.Position.Line, pe.Message)
		}
		return nil, err
	}
	if c.Settings.MenuMode == "" {
		c.Settings.MenuMode = "submenu"
	}
	if err := ValidateAll(c.Presets); err != nil {
		return nil, err
	}
	return c, nil
}

func Encode(c *Config) ([]byte, error) {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Load reads the config at path; if missing, creates it with defaults.
// A corrupt file is never overwritten.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c := Defaults()
		if err := Save(path, c); err != nil {
			return nil, err
		}
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func Save(path string, c *Config) error {
	if err := ValidateAll(c.Presets); err != nil {
		return err
	}
	data, err := Encode(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Defaults() *Config {
	v := func(codec string, q int, speed, scale string, w, h int, fps float64) *Video {
		return &Video{Codec: codec, Encoder: EncCPU, RateMode: RateQuality, Quality: q, Speed: speed, ScaleMode: scale, Width: w, Height: h, FPS: fps}
	}
	copyAudio := &Audio{Mode: AudioCopy}
	return &Config{
		Settings: Settings{MenuMode: "submenu", Integration: "classic"},
		Presets: []Preset{
			{ID: "stretch-fhd", Name: "4:3 → 16:9 (FHD)", Suffix: "stretch-fhd", Ext: "mp4", Video: v(CodecH264, 18, "medium", ScaleStretch, 1920, 1080, 0), Audio: copyAudio},
			{ID: "stretch-qhd", Name: "4:3 → 16:9 (QHD)", Suffix: "stretch-qhd", Ext: "mp4", Video: v(CodecH264, 18, "medium", ScaleStretch, 2560, 1440, 0), Audio: copyAudio},
			{ID: "volume-down", Name: i18n.T("Громкость −6 дБ"), Suffix: "volume-6db", Ext: "same", Video: &Video{Codec: CodecCopy}, Audio: &Audio{Mode: AudioAAC, Bitrate: "192k", VolumeDB: -6}},
			{ID: "fps-30", Name: "30 fps", Suffix: "30fps", Ext: "mp4", Video: v(CodecH264, 18, "medium", ScaleNone, 0, 0, 30), Audio: copyAudio},
			{ID: "scale-720p", Name: i18n.T("Разрешение 720p"), Suffix: "720p", Ext: "mp4", Video: v(CodecH264, 20, "medium", ScaleKeep, 0, 720, 0), Audio: copyAudio},
			{ID: "h265", Name: i18n.T("Перекодировать в H.265"), Suffix: "h265", Ext: "mp4", Video: v(CodecH265, 24, "medium", ScaleNone, 0, 0, 0), Audio: copyAudio},
		},
	}
}

// SplitArgs splits a line using shell-like quoting rules (single, double quotes, backslash in double quotes only before " or \).
func SplitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			in = true
		case r == ' ' || r == '\t' || r == '\n':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("%s", i18n.T("незакрытая кавычка"))
	}
	if in {
		out = append(out, cur.String())
	}
	return out, nil
}

// JoinArgs is the inverse of SplitArgs for display in the editor.
func JoinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			if strings.Contains(a, "\"") {
				parts[i] = "'" + a + "'"
			} else {
				parts[i] = "\"" + a + "\""
			}
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}

// DefaultMenuTitle is the Explorer menu item name unless the user sets another one. It follows the Windows display
// language (not the application language), in the style of "Edit with Notepad++".
func DefaultMenuTitle() string {
	if i18n.SystemLang() == i18n.RU {
		return "Обработать с помощью Soverter"
	}
	return "Convert with Soverter"
}

// MenuName returns the configured menu item name or the default.
func (s Settings) MenuName() string {
	if t := strings.TrimSpace(s.MenuTitle); t != "" {
		return t
	}
	return DefaultMenuTitle()
}
