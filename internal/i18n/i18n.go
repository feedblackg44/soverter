// Package i18n is a tiny translation layer. The source language of the code is Russian: T takes the Russian
// text and returns it as is for "ru" or the registered English translation for "en" (the default).
// A missing translation falls back to the Russian text, and TestCatalogComplete fails the build of the test
// suite if any literal passed to T has no English entry.
package i18n

import (
	"fmt"
	"sync"
)

// Supported languages.
const (
	EN = "en"
	RU = "ru"
)

var (
	mu   sync.RWMutex
	lang = EN
	en   = map[string]string{}
)

// Register adds Russian -> English translations (call from init).
func Register(m map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	for k, v := range m {
		en[k] = v
	}
}

// SetLang selects the UI language; anything but "ru" means English.
func SetLang(l string) {
	mu.Lock()
	defer mu.Unlock()
	if l == RU {
		lang = RU
	} else {
		lang = EN
	}
}

// Lang returns the current language code.
func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	return lang
}

// T translates s (Russian source text) and formats it with args if any.
func T(s string, args ...any) string {
	mu.RLock()
	if lang == EN {
		if v, ok := en[s]; ok {
			s = v
		}
	}
	mu.RUnlock()
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Has reports whether an English translation exists for s.
func Has(s string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := en[s]
	return ok
}

type lazyErr struct{ s string }

func (e *lazyErr) Error() string { return T(e.s) }

// Err returns a sentinel error whose text is translated when it is printed (safe for package-level vars).
func Err(s string) error { return &lazyErr{s} }

// N marks a Russian source text that is stored or passed around untranslated and translated later with T
// (for example cached data). It returns s unchanged; TestCatalogComplete checks that it has an English entry.
func N(s string) string { return s }
