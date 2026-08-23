// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package i18n

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// Catalog - This type holds one language's worth of translated strings,
// mapping each key to its text.
type Catalog map[string]string

// Translator - This type holds every loaded catalog, plus which language
// is currently active and which one to fall back to. defaultLang is set
// explicitly by whoever constructs the Translator, main.go, from the
// config's language directive, rather than guessed from convention, such
// as the first loaded catalog, so there is never ambiguity about which
// language text should fall back to.
type Translator struct {
	catalogs    map[string]Catalog
	currentLang string
	defaultLang string
}

// ----------------------------------------------------------------------
// Initialization Functions
// ----------------------------------------------------------------------

// New - This function constructs a Translator from a set of already
// loaded catalogs, see LoadCatalogs, plus which language should be active
// to start. An unrecognized language code in a config file makes for a
// much better startup experience if it falls back to defaultLang and
// starts in English than if it fails outright, so if lang is not among
// the loaded catalogs, New falls back to defaultLang instead of erroring.
// A caller that wants to know whether that happened should check
// CurrentLanguage() against what it asked for.
func New(catalogs map[string]Catalog, lang, defaultLang string) *Translator {
	t := &Translator{catalogs: catalogs, defaultLang: defaultLang}
	if _, ok := catalogs[lang]; ok {
		t.currentLang = lang
	} else {
		t.currentLang = defaultLang
	}
	return t
}
