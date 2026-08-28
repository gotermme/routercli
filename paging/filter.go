// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package paging

import (
	"regexp"
	"strings"
)

// ApplyFilters runs every stage in stages against lines, in order,
// each stage narrowing what the previous one already produced, the
// same left to right pipeline a shell's own "|" chain, or a real
// Cisco or HP device's own chained filter, already means. lines
// itself is never mutated; each stage's result is a new slice. mode
// chooses whether every stage's Pattern is matched as a plain
// substring or compiled as a regular expression, see FilterMode's own
// doc comment; the same mode applies to every stage in one call, a
// session cannot mix modes within one chained filter.
//
// The only error this returns is a regular expression that fails to
// compile, only possible when mode is FilterModeRegex. A substring
// match can never fail to compile, since there is nothing to compile.
func ApplyFilters(lines []string, stages []FilterStage, mode FilterMode) ([]string, error) {
	for _, stage := range stages {
		matcher, err := newMatcher(stage.Pattern, mode)
		if err != nil {
			return nil, err
		}
		lines = applyStage(lines, stage.Kind, matcher)
	}
	return lines, nil
}

// matchFunc - This type is a compiled pattern, built once per stage
// by newMatcher and then reused for every line that stage looks at,
// rather than recompiling a regular expression, or re-deriving a
// substring check, once per line.
type matchFunc func(line string) bool

// newMatcher - This function builds the matchFunc a single
// FilterStage's Pattern actually runs, once, before applyStage walks
// every line. See FilterMode's own doc comment for what each mode
// means.
func newMatcher(pattern string, mode FilterMode) (matchFunc, error) {
	if mode == FilterModeRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}
	return func(line string) bool { return strings.Contains(line, pattern) }, nil
}

// applyStage - This function is ApplyFilters' own per stage worker.
// Include and Exclude are each a plain, independent pass over every
// line. Begin finds the first matching line and keeps it and
// everything after it, discarding everything before, or the whole
// slice when nothing matches at all, matching real Cisco and HP
// behavior for a "begin" pattern that never appears in the output.
func applyStage(lines []string, kind FilterKind, matches matchFunc) []string {
	switch kind {
	case FilterExclude:
		var out []string
		for _, line := range lines {
			if !matches(line) {
				out = append(out, line)
			}
		}
		return out

	case FilterBegin:
		for i, line := range lines {
			if matches(line) {
				return lines[i:]
			}
		}
		return nil

	default: // FilterInclude
		var out []string
		for _, line := range lines {
			if matches(line) {
				out = append(out, line)
			}
		}
		return out
	}
}
