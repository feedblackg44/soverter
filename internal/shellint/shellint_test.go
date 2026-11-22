package shellint

import (
	"strings"
	"testing"

	"vidpreset/internal/preset"
)

type mock struct {
	vals    map[string]string
	deleted []string
	pruned  []string
}

func (m *mock) SetString(k, n, v string) error { m.vals[k+"|"+n] = v; return nil }
func (m *mock) DeleteTree(k string) error      { m.deleted = append(m.deleted, k); return nil }
func (m *mock) DeleteIfEmpty(k string) error   { m.pruned = append(m.pruned, k); return nil }

var ps = []preset.Preset{
	{ID: "b-one", Name: "Один"},
	{ID: "a-two", Name: "Два"},
}

func TestInstallSubmenu(t *testing.T) {
	m := &mock{vals: map[string]string{}}
	if err := Install(m, `C:\Tools\vid preset.exe`, ps, "submenu", ""); err != nil {
		t.Fatal(err)
	}
	root := RootFor("mp4")
	if len(m.deleted) != len(Extensions)+1 {
		t.Fatalf("deleted %v", m.deleted)
	}
	if v, ok := m.vals[root+"|SubCommands"]; !ok || v != "" {
		t.Fatal("SubCommands missing")
	}
	for _, ext := range Extensions {
		if _, ok := m.vals[RootFor(ext)+"|MUIVerb"]; !ok {
			t.Fatalf("no menu for .%s", ext)
		}
	}
	k := root + `\shell\001-b-one`
	if m.vals[k+"|MUIVerb"] != "Один" {
		t.Fatal("cyrillic name")
	}
	if got := m.vals[k+`\command|`]; got != `"C:\Tools\vid preset.exe" run b-one "%1"` {
		t.Fatal(got)
	}
	if _, ok := m.vals[root+`\shell\002-a-two|MUIVerb`]; !ok {
		t.Fatal("order prefix")
	}
	for key := range m.vals {
		if !IsSafeRoot(strings.SplitN(key, "|", 2)[0]) {
			t.Fatalf("write outside our roots: %s", key)
		}
	}
}

func TestInstallSingle(t *testing.T) {
	m := &mock{vals: map[string]string{}}
	_ = Install(m, `C:\v.exe`, ps, "single", "")
	root := RootFor("mkv")
	if got := m.vals[root+`\command|`]; got != `"C:\v.exe" pick "%1"` {
		t.Fatal(got)
	}
	if _, ok := m.vals[root+"|SubCommands"]; ok {
		t.Fatal("no SubCommands in single mode")
	}
}

func TestUninstall(t *testing.T) {
	m := &mock{vals: map[string]string{}}
	_ = Uninstall(m)
	if len(m.deleted) != len(Extensions)+1 {
		t.Fatal(m.deleted)
	}
	for _, k := range m.pruned {
		if !IsSafePrune(k) {
			t.Fatalf("unsafe prune %s", k)
		}
	}
	for _, k := range m.deleted {
		if !IsSafeRoot(k) {
			t.Fatalf("unsafe delete %s", k)
		}
	}
	if IsSafeRoot(classes) || IsSafePrune(classes) || IsSafePrune(classes+`\video`) {
		t.Fatal("shared keys must never be touched")
	}
}

func TestDisabledSkipped(t *testing.T) {
	m := &mock{vals: map[string]string{}}
	list := []preset.Preset{{ID: "off", Name: "Off", Disabled: true}, {ID: "on", Name: "On"}}
	_ = Install(m, `C:\v.exe`, list, "submenu", "")
	if _, ok := m.vals[RootFor("mp4")+`\shell\001-on|MUIVerb`]; !ok {
		t.Fatal("enabled preset must be 001")
	}
	for k := range m.vals {
		if strings.Contains(k, "off") {
			t.Fatalf("disabled preset registered: %s", k)
		}
	}
}

func TestMenuTitle(t *testing.T) {
	ps := []preset.Preset{{ID: "a", Name: "A", Suffix: "a", Ext: "mp4", VF: "x"}}
	find := func(title string) string {
		for _, e := range Plan(`C:\v.exe`, ps, "submenu", title) {
			if e.Name == "MUIVerb" && e.Key == RootFor("mp4") {
				return e.Value
			}
		}
		return "?"
	}
	if got := find(""); got != "vidpreset" {
		t.Fatalf("default title %q", got)
	}
	if got := find("My videos"); got != "My videos" {
		t.Fatalf("custom title %q", got)
	}
}
