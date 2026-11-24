package gui

import (
	"testing"

	"soverter/internal/preset"
)

func TestThemeModeSwitchesPalette(t *testing.T) {
	defer SetThemeMode(ThemeSystem)
	SetThemeMode(ThemeLight)
	if Theme().P.Dark || ThemeMode() != ThemeLight {
		t.Fatalf("light: dark=%v mode=%q", Theme().P.Dark, ThemeMode())
	}
	before := Theme()
	SetThemeMode(ThemeDark)
	if !Theme().P.Dark || ThemeMode() != ThemeDark {
		t.Fatalf("dark: dark=%v mode=%q", Theme().P.Dark, ThemeMode())
	}
	if Theme() == before {
		t.Fatal("a window must see a new theme pointer to retint its title bar")
	}
	if Theme().Shaper != before.Shaper {
		t.Fatal("fonts must be shared, not reloaded on every switch")
	}
}

func TestConfigureThemeFromSettings(t *testing.T) {
	defer SetThemeMode(ThemeSystem)
	Configure(preset.Settings{Theme: ThemeLight})
	if ThemeMode() != ThemeLight {
		t.Fatalf("mode = %q", ThemeMode())
	}
	Configure(preset.Settings{})
	if ThemeMode() != ThemeSystem {
		t.Fatalf("empty setting must mean system, got %q", ThemeMode())
	}
}
