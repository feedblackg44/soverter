package preset

import (
	"reflect"
	"strings"
	"testing"
)

func TestDefaultsValidAndRoundTrip(t *testing.T) {
	c := Defaults()
	if err := ValidateAll(c.Presets); err != nil {
		t.Fatal(err)
	}
	data, err := Encode(c)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Presets, c2.Presets) {
		t.Fatal("round trip mismatch")
	}
}

func TestValidate(t *testing.T) {
	ok := Preset{ID: "a-1", Name: "n", Suffix: "s", Ext: "mp4", VF: "x"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Preset{
		"upper id":  {ID: "A", Name: "n", Suffix: "s", Ext: "mp4", VF: "x"},
		"space id":  {ID: "a b", Name: "n", Suffix: "s", Ext: "mp4", VF: "x"},
		"empty id":  {Name: "n", Suffix: "s", Ext: "mp4", VF: "x"},
		"no name":   {ID: "a", Suffix: "s", Ext: "mp4", VF: "x"},
		"no suffix": {ID: "a", Name: "n", Ext: "mp4", VF: "x"},
		"no ext":    {ID: "a", Name: "n", Suffix: "s", VF: "x"},
		"no action": {ID: "a", Name: "n", Suffix: "s", Ext: "mp4"},
	}
	for n, p := range bad {
		if p.Validate() == nil {
			t.Errorf("%s: expected error", n)
		}
	}
	if ValidateAll([]Preset{ok, ok}) == nil {
		t.Error("duplicate id must fail")
	}
}

func TestParseErrors(t *testing.T) {
	_, err := Parse([]byte("[[preset]]\nid = \"a\"\nname = \n"))
	if err == nil || !strings.Contains(err.Error(), "строке 3") {
		t.Fatalf("want line number, got %v", err)
	}
	_, err = Parse([]byte("[[preset]]\nid = \"Bad Id\"\nname=\"n\"\nsuffix=\"s\"\next=\"mp4\"\nvf=\"x\"\n"))
	if err == nil {
		t.Fatal("bad id must fail")
	}
}

func TestSplitJoinArgs(t *testing.T) {
	got, err := SplitArgs(`-c:v libx264 -metadata "title=a b" -x 'q r'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-c:v", "libx264", "-metadata", "title=a b", "-x", "q r"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	back, _ := SplitArgs(JoinArgs(want))
	if !reflect.DeepEqual(back, want) {
		t.Fatalf("join/split %q", back)
	}
	if _, err := SplitArgs(`"open`); err == nil {
		t.Fatal("unclosed quote must fail")
	}
}
