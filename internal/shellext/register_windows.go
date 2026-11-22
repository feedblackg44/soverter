package shellext

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"vidpreset/internal/i18n"

	"golang.org/x/sys/windows/registry"

	"vidpreset/internal/preset"
)

// WorkDir holds the extracted package file (not under AppData; kept simple and visible).
func WorkDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".vidpreset", "shellext"), nil
}

const repoKey = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

// Registered reports whether the package is registered for the current user (cheap registry check).
func Registered() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, repoKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return false
	}
	for _, n := range names {
		if strings.HasPrefix(n, PackageName+"_") {
			return true
		}
	}
	return false
}

func powershell(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command",
		"$ErrorActionPreference='Stop'; "+script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func writeMenu(exe string, presets []preset.Preset, mode string) error {
	dir, err := preset.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "menu.txt"), MenuFile(exe, presets, mode), 0o644)
}

// CertTrusted reports whether the embedded self-signed certificate is in LocalMachine\TrustedPeople.
// True when there is no embedded certificate (package signed by a certificate Windows trusts on its own).
func CertTrusted() bool {
	th := CertThumbprint()
	if th == "" {
		return true
	}
	_, err := powershell(fmt.Sprintf(`if (-not (Test-Path 'Cert:\LocalMachine\TrustedPeople\%s')) { exit 1 }`, th))
	return err == nil
}

// TrustCert imports the embedded certificate into LocalMachine\TrustedPeople. This needs administrator
// rights, so it triggers a UAC prompt.
func TrustCert() error {
	der := certBytes()
	if der == nil {
		return nil
	}
	dir, err := WorkDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cer := filepath.Join(dir, "vidpreset.cer")
	if err := os.WriteFile(cer, der, 0o644); err != nil {
		return err
	}
	inner := fmt.Sprintf(`Import-Certificate -FilePath '%s' -CertStoreLocation Cert:\LocalMachine\TrustedPeople | Out-Null`, cer)
	_, err = powershell(fmt.Sprintf(`Start-Process powershell -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-Command',"%s"`,
		strings.ReplaceAll(inner, `"`, "`\"")))
	if err != nil {
		return errors.New(i18n.T("не удалось запросить права администратора для установки сертификата (отмена UAC?)"))
	}
	if !CertTrusted() {
		return errors.New(i18n.T("сертификат не был добавлен в доверенные (подтвердите запрос UAC)"))
	}
	return nil
}

// UntrustCert removes the embedded certificate from LocalMachine\TrustedPeople (UAC prompt).
func UntrustCert() error {
	th := CertThumbprint()
	if th == "" {
		return nil
	}
	inner := fmt.Sprintf(`Remove-Item 'Cert:\LocalMachine\TrustedPeople\%s' -ErrorAction SilentlyContinue`, th)
	_, err := powershell(fmt.Sprintf(`Start-Process powershell -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-Command',"%s"`, inner))
	if err != nil {
		return errors.New(i18n.T("не удалось запросить права администратора (отмена UAC?)"))
	}
	return nil
}

// Install writes menu.txt and (if needed) registers the package. Safe to call after every preset change.
func Install(exe string, presets []preset.Preset, mode string) error {
	if len(msixBytes()) == 0 {
		return errors.New(i18n.T("в сборке нет пакета меню Windows 11: запустите shellext\\package.ps1 и пересоберите exe"))
	}
	if err := writeMenu(exe, presets, mode); err != nil {
		return err
	}
	dir, err := WorkDir()
	if err != nil {
		return err
	}
	verFile := filepath.Join(dir, "version.txt")
	if have, _ := os.ReadFile(verFile); Registered() && string(have) == Version() {
		return nil // already registered with this package build
	}
	if Registered() {
		if err := removePackage(); err != nil {
			return err
		}
	}
	if !CertTrusted() {
		if err := TrustCert(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	msix := filepath.Join(dir, "vidpreset.msix")
	if err := os.WriteFile(msix, msixBytes(), 0o644); err != nil {
		return err
	}
	if out, err := powershell(fmt.Sprintf(`Add-AppxPackage -Path '%s'`, msix)); err != nil {
		return errors.New(i18n.T("не удалось зарегистрировать меню Windows 11:\n") + out)
	}
	return os.WriteFile(verFile, []byte(Version()), 0o644)
}

func removePackage() error {
	out, err := powershell(fmt.Sprintf(`Get-AppxPackage -Name '%s' | Remove-AppxPackage`, PackageName))
	if err != nil {
		return fmt.Errorf(i18n.T("не удалось удалить пакет меню: %s"), out)
	}
	return nil
}

// Uninstall unregisters the package and removes our files (the trusted certificate stays; see UntrustCert).
func Uninstall() error {
	if Registered() {
		if err := removePackage(); err != nil {
			return err
		}
	}
	if dir, err := WorkDir(); err == nil {
		_ = os.RemoveAll(dir)
	}
	if d, err := preset.Dir(); err == nil {
		_ = os.Remove(filepath.Join(d, "menu.txt"))
	}
	return nil
}
