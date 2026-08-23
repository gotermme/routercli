// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadCatalogs - This function reads every *.yaml file in dir and
// returns them as a map keyed by language code, taken from each file's
// base name, so var/lang/en.yaml becomes "en" and var/lang/fr.yaml
// becomes "fr". Each file is a flat map of key to text, with no nested
// structure and no wrapper key repeating the language code the filename
// already states.
//
// A missing directory is not an error. It returns an empty catalog set,
// the same reasoning as the missing file handling in
// config.LoadToolConfig. i18n is opt-in, and a project that has not set
// up var/lang/ yet should still run, with every T() call falling back to
// its raw key, which is visibly obvious rather than a startup crash over
// a feature nobody is using yet. A file that exists but fails to parse
// is a hard error, though, since a malformed YAML catalog is a real
// mistake worth surfacing immediately rather than silently dropping
// that language.
func LoadCatalogs(dir string) (map[string]Catalog, error) {
	catalogs := make(map[string]Catalog)

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return catalogs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading language directory %q: %v", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ".yaml")
		path := filepath.Join(dir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("error reading language file %q: %v", path, err)
		}

		var cat Catalog
		if err := yaml.Unmarshal(data, &cat); err != nil {
			return nil, fmt.Errorf("error parsing language file %q: %v", path, err)
		}
		catalogs[lang] = cat
	}

	return catalogs, nil
}
