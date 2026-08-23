// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// ----------------------------------------------------------------------
// Public Functions - Loader
// ----------------------------------------------------------------------

// LoadTree - This function reads a command tree from a YAML file at
// path and resolves every "run:" directive against the plugin registry,
// see registry.go. A command that silently does nothing because of a
// typo in a tree file would be a much worse failure mode than refusing
// to start, so this fails loudly and immediately if the file cannot be
// read, cannot be parsed, references a handler name that no command
// file actually registered, or sets a property name this package does
// not recognize. This is the loader half of the plugin design. Command
// behavior lives in package cmd files that self-register through
// init(), while command structure, what is callable, what it is named,
// and what it requires, lives here in data.
//
// Unknown YAML keys are a hard error, the same way config.LoadSystemConfig
// and auth.LoadUsers treat them for their own files. A misspelled
// property name in a tree file would otherwise be silently dropped
// rather than erroring, leaving a command missing a directive its
// author thought they had set. A tree file with more than one YAML
// document is also a hard error, the same way those two loaders treat
// it, since a command tree file is expected to be a single top-level
// mapping.
func LoadTree(path string) (map[string]*Command, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading command tree file %q: %v", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var raw commandTreeFile
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			// The file is empty, or contains comments only. An empty
			// tree, zero commands, is unusual but not itself an error.
			return nil, nil
		}
		return nil, fmt.Errorf("error parsing command tree file %q: %v", path, err)
	}

	var extra commandTreeFile
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("command tree file %q contains multiple YAML documents", path)
		}
		return nil, fmt.Errorf("error parsing command tree file %q: %v", path, err)
	}

	if err := resolveHandlers(raw.Commands, ""); err != nil {
		return nil, fmt.Errorf("error building command tree from %q: %v", path, err)
	}
	return raw.Commands, nil
}

// ----------------------------------------------------------------------
// Private Functions - Loader
// ----------------------------------------------------------------------

// resolveHandlers - This function recursively resolves every command's
// Run into its actual RunFunc handler by looking it up in the plugin
// registry. Every other field on Command already came through decoding
// directly, see Command's own doc comment, so this is the one thing
// left for a tree to do after yaml.Unmarshal returns. pathSoFar exists
// so an error message can point at the actual failing command instead
// of just naming a handler string with no context for where it went
// wrong. A command with an empty Run, a pure container command
// with only Subcommands, is left with a nil RunFunc, exactly as intended.
func resolveHandlers(tree map[string]*Command, pathSoFar string) error {
	for name, c := range tree {
		commandPath := name
		if pathSoFar != "" {
			commandPath = pathSoFar + " " + name
		}

		if c.Run != "" {
			fn, ok := lookupHandler(c.Run)
			if !ok {
				return fmt.Errorf("command %q references handler %q, but nothing registered that name (check the command's init() call)", commandPath, c.Run)
			}
			c.RunFunc = fn
		}

		if err := resolveHandlers(c.Subcommands, commandPath); err != nil {
			return err
		}
	}
	return nil
}
