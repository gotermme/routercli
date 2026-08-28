// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package paging

import (
	"fmt"
	"strings"
)

// SplitPipeline splits tokens, an already tokenized command line, see
// tokenize.Tokenize, on every bare "|" token, the same convention a
// shell pipeline uses. The first group, cmdTokens, is everything
// before the first "|" and is what a caller resolves against a
// command tree, see command.Resolve. Every group after it becomes one
// entry of segments, in order, one raw, still unparsed, filter stage
// each. A line with no "|" token at all returns tokens unchanged as
// cmdTokens and a nil segments, so a caller can check len(segments)
// == 0 to know no filtering was requested.
//
// This is a purely syntactic split. It does not know whether a
// segment is well formed, "include", "exclude", or "begin" followed
// by a pattern, only where one segment ends and the next begins. See
// ParseStages for turning a segment into a real FilterStage.
func SplitPipeline(tokens []string) (cmdTokens []string, segments [][]string) {
	var groups [][]string
	var current []string
	for _, t := range tokens {
		if t == "|" {
			groups = append(groups, current)
			current = nil
			continue
		}
		current = append(current, t)
	}
	groups = append(groups, current)

	if len(groups) == 1 {
		return groups[0], nil
	}
	return groups[0], groups[1:]
}

// ParseStages turns segments, the raw filter groups SplitPipeline
// already split out, into a real []FilterStage, ready for
// ApplyFilters. maxDepth is
// config.SystemConfig.MaxFilterChainDepth's own configured value,
// checked here rather than left to whatever ApplyFilters would do
// with an unbounded chain, since an operator typing too many filters
// is a mistake to report clearly, not something to silently truncate
// or run anyway. maxDepth of zero means filtering is disabled
// entirely for this deployment, and any segments at all, even one, is
// refused.
//
// Each segment's own first token MUST be exactly "include",
// "exclude", or "begin", case sensitive, matching every other literal
// command word this project resolves. Everything after that first
// token, however many tokens SplitPipeline produced for it, is joined
// back together with a single space to form Pattern, so a pattern
// containing spaces needs no special quoting beyond whatever
// tokenize.Tokenize itself already requires, "include \"eth 0\"" and
// "include eth 0" both produce the same pattern, "eth 0", matching
// how a real Cisco or HP device reads the rest of the line verbatim
// once it sees "include". A segment with no token at all, an empty
// or a doubled "|", or with a keyword and no pattern following it, is
// a real error, not silently ignored.
//
// ParseStages never itself checks whether Pattern is a valid regular
// expression. FilterMode is chosen at the point a stage actually
// runs, see ApplyFilters, since the same parsed stage list is reused
// unchanged regardless of which mode is active when it is applied.
func ParseStages(segments [][]string, maxDepth int) ([]FilterStage, error) {
	if len(segments) == 0 {
		return nil, nil
	}

	if maxDepth <= 0 {
		return nil, fmt.Errorf("output filtering is disabled")
	}

	if len(segments) > maxDepth {
		return nil, fmt.Errorf("too many filters (%d), the maximum is %d", len(segments), maxDepth)
	}

	stages := make([]FilterStage, 0, len(segments))
	for _, segment := range segments {
		if len(segment) == 0 {
			return nil, fmt.Errorf("empty filter, expected 'include', 'exclude', or 'begin' followed by a pattern")
		}
		if len(segment) < 2 {
			return nil, fmt.Errorf("filter %q has no pattern to match against", segment[0])
		}

		var kind FilterKind
		switch segment[0] {
		case "include":
			kind = FilterInclude
		case "exclude":
			kind = FilterExclude
		case "begin":
			kind = FilterBegin
		default:
			return nil, fmt.Errorf("unknown filter %q, expected 'include', 'exclude', or 'begin'", segment[0])
		}

		stages = append(stages, FilterStage{Kind: kind, Pattern: strings.Join(segment[1:], " ")})
	}
	return stages, nil
}
