// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package tokenize implements a quote-aware tokenizer in the style of a
shell, along with its inverse, so a value containing spaces can round
trip through both a command line and printed output unchanged. See
Tokenize and QuoteIfNeeded.
*/
package tokenize
