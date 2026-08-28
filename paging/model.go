// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package paging

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// FilterMode - This type chooses how a FilterStage's Pattern is
// matched against one line of output. See FilterModeSubstring and
// FilterModeRegex.
type FilterMode int

const (
	// FilterModeSubstring matches a line whenever it literally
	// contains Pattern, with no special characters of any kind. This
	// is the default, config.SystemConfig.FilterMatchMode's own
	// default value, since it is predictable for an operator who
	// just wants a plain word search and never needs to think about
	// escaping a character that happens to also be a regular
	// expression metacharacter, a period in an IP address for
	// example.
	FilterModeSubstring FilterMode = iota

	// FilterModeRegex matches a line whenever Pattern, compiled as a
	// Go RE2 regular expression, see the regexp package, finds a
	// match anywhere in it. This is what real Cisco and HP devices
	// actually do, so a project that wants exact vendor parity
	// switches to this mode, either through
	// config.SystemConfig.FilterMatchMode at startup or through
	// "terminal filter-mode regex" at runtime.
	FilterModeRegex
)

// FilterKind - This type names which of the three pipe filter
// keywords, "include", "exclude", or "begin", one FilterStage
// applies. See ApplyFilters for what each one actually does to a
// list of lines.
type FilterKind int

const (
	// FilterInclude keeps only a line that matches the pattern,
	// discarding every other line.
	FilterInclude FilterKind = iota

	// FilterExclude keeps only a line that does not match the
	// pattern, discarding every line that does.
	FilterExclude

	// FilterBegin discards every line before the first one that
	// matches the pattern, then keeps that line and everything after
	// it. When no line matches at all, the result is empty, matching
	// real Cisco and HP behavior for a "begin" pattern that never
	// appears in the output.
	FilterBegin
)

// FilterStage - This type is one stage of a pipe filter pipeline, one
// "| include eth0" or "| begin interface" segment of a typed command
// line. ApplyFilters runs a list of these against a command's
// captured output, in order, each stage narrowing what the previous
// one already produced.
type FilterStage struct {
	Kind    FilterKind
	Pattern string
}
