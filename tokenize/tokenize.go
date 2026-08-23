// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package tokenize

import (
	"fmt"
	"strings"
)

// Tokenize - This function splits a line of input into individual tokens
// the same way a shell would, honoring single and double quotes so that
// a value such as a description can contain spaces. This matters for
// two related reasons. First, a command such as set description
// "uplink to core" needs to parse as three tokens, set, description,
// and uplink to core, not six. Second, whatever "show running-config"
// prints back out needs to paste back into the console and tokenize to
// the exact same values it started from, which is impossible for any
// value containing a space without quote awareness.
//
// Unquoted whitespace separates tokens. A token wrapped in matching
// single or double quotes becomes one token, with the quotes themselves
// stripped, and the quote character can be escaped inside a
// double-quoted token with a backslash. An unterminated quote is reported as
// an error rather than silently guessed at, since silently guessing is
// exactly how a config round trip goes quietly wrong.
func Tokenize(line string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inToken := false

	runes := []rune(line)
	i := 0
	for i < len(runes) {
		r := runes[i]

		switch {
		case r == ' ' || r == '\t':
			if inToken {
				tokens = append(tokens, current.String())
				current.Reset()
				inToken = false
			}
			i++

		case r == '"' || r == '\'':
			quote := r
			inToken = true
			i++
			closed := false
			for i < len(runes) {
				if runes[i] == '\\' && quote == '"' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\\') {
					current.WriteRune(runes[i+1])
					i += 2
					continue
				}
				if runes[i] == quote {
					closed = true
					i++
					break
				}
				current.WriteRune(runes[i])
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated %c quote in input", quote)
			}

		default:
			inToken = true
			current.WriteRune(r)
			i++
		}
	}

	if inToken {
		tokens = append(tokens, current.String())
	}

	return tokens, nil
}

// QuoteIfNeeded - This function is what makes "show running-config"
// output pasteable back into the console. It is the inverse of
// Tokenize. Given a single value, it returns a string that Tokenize()
// will parse back into exactly that value. A value with no whitespace,
// quote, or backslash character is returned unchanged, so a simple
// command stays readable and undecorated. Anything containing one of
// those is wrapped in double quotes with internal backslashes and
// double quotes escaped.
//
// Backslashes are escaped first, before quotes. Reversing the order
// would double-escape the backslashes that Tokenize's own handling of
// \" relies on, and silently corrupt any value that contains one. For
// example, a value ending in a literal backslash, such as `path C:\`,
// needs to become `"path C:\\"` so it round trips correctly. Escaping
// the quote first instead would produce `"path C:\"`, which Tokenize
// would misread as an escaped quote followed by an unterminated
// string, rather than a literal backslash followed by the real closing
// quote.
func QuoteIfNeeded(value string) string {
	if value == "" {
		return `""`
	}
	if !strings.ContainsAny(value, " \t\"'\\") {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
