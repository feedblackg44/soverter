package shellint

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// WinRegistry implements Registry on HKCU.
type WinRegistry struct{}

func (WinRegistry) SetString(key, name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (WinRegistry) DeleteTree(key string) error {
	if !IsSafeRoot(key) {
		return fmt.Errorf("refusing to delete %s", key)
	}
	return deleteTree(key)
}

func deleteTree(key string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	subs, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := deleteTree(key + `\` + s); err != nil {
			return err
		}
	}
	err = registry.DeleteKey(registry.CURRENT_USER, key)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// Installed reports whether our menu key exists for .mp4.
func Installed() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, RootFor("mp4"), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

var _ = strings.TrimSpace

func (WinRegistry) DeleteIfEmpty(key string) error {
	if !IsSafePrune(key) {
		return fmt.Errorf("refusing to prune %s", key)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := k.Stat()
	k.Close()
	if err != nil {
		return err
	}
	if info.SubKeyCount != 0 || info.ValueCount != 0 {
		return nil
	}
	err = registry.DeleteKey(registry.CURRENT_USER, key)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
