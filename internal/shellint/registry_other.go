//go:build !windows

package shellint

import "errors"

// WinRegistry is a stub on non-Windows platforms.
type WinRegistry struct{}

func (WinRegistry) SetString(key, name, value string) error {
	return errors.New(i18n.T("только Windows"))
}
func (WinRegistry) DeleteTree(key string) error { return errors.New(i18n.T("только Windows")) }

func Installed() bool { return false }

func (WinRegistry) DeleteIfEmpty(key string) error { return errors.New(i18n.T("только Windows")) }
