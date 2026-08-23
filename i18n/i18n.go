// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package i18n

import (
	"fmt"
	"sort"
)

// ----------------------------------------------------------------------
// Public Methods
// ----------------------------------------------------------------------

// T - This method looks up key in the current language, falls back to
// the default language, and falls back to the literal key wrapped in
// double brackets, "[[key]]", if neither has it, so a missing
// translation is something a person notices immediately in the CLI's
// output rather than something they have to go hunting for in a log
// file. args are applied with fmt.Sprintf if any are given, so a catalog
// entry can contain a verb such as %s or %d the same way any other Go
// format string would.
//
// T is safe to call on a nil *Translator, returning the bracketed key as
// if no catalogs were loaded at all. This lets a caller throughout the
// project use ctx.Translator.T(...) unconditionally, with no call site
// needing its own nil check for the valid case where i18n was never
// wired up at all.
func (t *Translator) T(key string, args ...any) string {
	if t == nil {
		return "[[" + key + "]]"
	}
	text, ok := t.lookup(t.currentLang, key)
	if !ok {
		text, ok = t.lookup(t.defaultLang, key)
	}
	if !ok {
		// Not found anywhere, so return the bracketed key as is. args
		// is deliberately not applied here. The placeholder has no
		// format verbs of its own, and running it through Sprintf
		// would just tack on an ugly "%!(EXTRA ...)" suffix instead of
		// the clean, obviously missing translation marker this is
		// meant to be.
		return "[[" + key + "]]"
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

func (t *Translator) lookup(lang, key string) (string, bool) {
	cat, ok := t.catalogs[lang]
	if !ok {
		return "", false
	}
	text, ok := cat[key]
	return text, ok
}

// SetLanguage - This method switches the active language, used by the
// runtime "language set" command, cmd/cmd_language.go. Falling back
// silently to a default is fine as a constructor default, see New's own
// doc comment, but it is wrong for an explicit runtime request, where
// the user needs to be told either that they got what they asked for or
// why not. So SetLanguage returns an error naming the requested language
// if it was not loaded, rather than falling back, letting the command
// handler report a useful message.
func (t *Translator) SetLanguage(lang string) error {
	if _, ok := t.catalogs[lang]; !ok {
		return fmt.Errorf("language %q is not loaded (available: %v)", lang, t.AvailableLanguages())
	}
	t.currentLang = lang
	return nil
}

// CurrentLanguage - This method returns the active language code.
func (t *Translator) CurrentLanguage() string {
	return t.currentLang
}

// AvailableLanguages - This method returns every loaded language code,
// sorted for stable output since Go's map iteration order is randomized,
// for the "language" command to list and for the error message in
// SetLanguage.
func (t *Translator) AvailableLanguages() []string {
	langs := make([]string, 0, len(t.catalogs))
	for l := range t.catalogs {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	return langs
}
