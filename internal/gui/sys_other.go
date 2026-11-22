//go:build !windows

package gui

func systemLight() bool { return false }

// PickVideoFile is only implemented on Windows.
func PickVideoFile(title string) (string, bool) { return "", false }
