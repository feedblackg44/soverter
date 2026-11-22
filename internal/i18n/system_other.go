//go:build !windows

package i18n

// SystemLang is only meaningful on Windows.
func SystemLang() string { return EN }
