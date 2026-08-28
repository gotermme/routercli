// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package paging implements output filtering and pagination in the
style of a real Cisco or HP ProCurve device, "show running-config |
include eth0" and a "--More--" pause for output longer than one
screen. Nothing here knows about command trees, handler functions, or
i18n keys beyond the translator it is handed. It works entirely in
terms of already produced lines of text, so it can sit between any
command's output and the real terminal with no dependency on package
command or either implementation package, cmd/core or cmd/product. See
SplitPipeline and ParseStages for turning
typed tokens into a filter pipeline, ApplyFilters for running that
pipeline against a command's captured output, CaptureOutput for
capturing that output in the first place, and Display for the
interactive pager itself.
*/
package paging
