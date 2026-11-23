package shellext

import (
	"archive/zip"
	"bytes"
	"crypto/x509"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"

	"soverter/internal/preset"
	"soverter/internal/shellint"
)

func readZip(t *testing.T) map[string][]byte {
	t.Helper()
	b := msixBytes()
	if len(b) == 0 {
		t.Skip("assets/soverter.msix is not built (it is not committed); run shellext/package.ps1")
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = data
	}
	return files
}

func TestPackageContents(t *testing.T) {
	files := readZip(t)
	for _, n := range []string{"AppxManifest.xml", "AppxSignature.p7x", "AppxBlockMap.xml", "soverter_ctx.dll", "stub.exe", "logo.png"} {
		if len(files[n]) == 0 {
			t.Errorf("package lacks %s", n)
		}
	}
	m := string(files["AppxManifest.xml"])
	d := xml.NewDecoder(strings.NewReader(m))
	for {
		if _, err := d.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(m, "@PUBLISHER@") || strings.Contains(m, "@VERSION@") {
		t.Fatal("manifest placeholders not substituted")
	}
	for _, e := range shellint.Extensions {
		if !strings.Contains(m, `Type=".`+e+`"`) {
			t.Errorf("no item type for .%s", e)
		}
	}
	if string(files["soverter_ctx.dll"][:2]) != "MZ" {
		t.Fatal("dll is not a PE file")
	}
}

func TestCertMatchesPublisher(t *testing.T) {
	der := certBytes()
	if der == nil {
		t.Skip("package signed with a real certificate; no embedded .cer")
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	m := string(readZip(t)["AppxManifest.xml"])
	if !strings.Contains(m, `Publisher="`+c.Subject.String()+`"`) {
		t.Fatalf("manifest publisher does not match cert subject %q", c.Subject.String())
	}
	if len(CertThumbprint()) != 40 {
		t.Fatal("thumbprint")
	}
}

func TestMenuFile(t *testing.T) {
	ps := []preset.Preset{
		{ID: "a", Name: "Первый\tпункт"},
		{ID: "off", Name: "Off", Disabled: true},
		{ID: "b", Name: "Second"},
	}
	got := string(MenuFile(`C:\Tools\vid preset.exe`, ps, "submenu", ""))
	want := "exe=C:\\Tools\\vid preset.exe\nmode=submenu\na\tПервый пункт\nb\tSecond\n"
	if got != want {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(string(MenuFile("x", nil, "single", "")), "mode=single") {
		t.Fatal("single mode")
	}
	if s := string(MenuFile("x", nil, "submenu", " My videos\n")); !strings.Contains(s, "title=My videos\n") {
		t.Fatalf("title line missing or not sanitized: %q", s)
	}
}

// Live registration test; only with VP_LIVE=1 (changes package registrations and, for a self-signed build,
// triggers a UAC prompt to trust the certificate).
func TestLiveRegister(t *testing.T) {
	if os.Getenv("VP_LIVE") != "1" {
		t.Skip("set VP_LIVE=1")
	}
	exe, _ := os.Executable()
	if err := Install(exe, preset.Defaults().Presets, "submenu", ""); err != nil {
		t.Fatal(err)
	}
	if !Registered() {
		t.Fatal("not registered after Install")
	}
	if err := Uninstall(); err != nil {
		t.Fatal(err)
	}
	if Registered() {
		t.Fatal("still registered")
	}
}
