package i18n

import "syscall"

// SystemLang returns "ru" if the Windows display language is Russian, otherwise "en". It is independent of the
// application's own language setting: Explorer menu entries should match the language of the system.
func SystemLang() string {
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	if r&0x3ff == 0x19 { // LANG_RUSSIAN
		return RU
	}
	return EN
}
