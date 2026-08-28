// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package paging

import (
	"bufio"
	"bytes"
	"io"
	"os"
)

// CaptureOutput runs fn with the real, process wide os.Stdout
// temporarily redirected to an in-memory pipe, and returns everything
// fn printed, split into individual lines. os.Stdout is always
// restored before this returns, success or panic, since fn is a real
// command handler and the rest of this project's own output must
// keep reaching the real terminal once fn is done.
//
// Only a Pageable command is ever run through this. See
// command.Command.Pageable's own doc comment for why: an interactive
// handler that prompts for a masked password or a confirmation code
// mid-command, "totp enable" or "password change" for instance, would
// have its own prompt silently swallowed into this buffer instead of
// reaching the real terminal where a person needs to see it before
// typing a blind response, so a Pageable command MUST never itself
// read from the terminal partway through RunFunc. Every command that
// does is deliberately left unmarked, running exactly as it always
// has, direct, unbuffered, fully interactive, with no dependency on
// this function at all.
//
// The pipe's own read end is drained on a separate goroutine, running
// concurrently with fn, rather than only after fn returns. This is
// required for correctness, not merely faster: a real os.Pipe's
// kernel buffer is a fixed, fairly small size, 64 KiB on a typical
// Linux system, and a single command whose own output exceeds that in
// one write, "show running-config" on a device with a very large
// configuration for instance, would otherwise block fn's own
// fmt.Println call forever, waiting for a reader this function would
// not even start until fn had already returned, a genuine deadlock
// every time it happened, not just a slow path.
//
// The line splitting itself uses bufio.Scanner's own ScanLines logic,
// which drops a trailing newline correctly rather than producing a
// spurious empty final line the way a naive strings.Split on "\n"
// would for any output that, like nearly everything this project
// prints, ends in one.
//
// The only error this returns is a failure to construct the
// underlying os.Pipe, or a failure while reading from or closing it,
// each essentially impossible on a real system, kept here rather than
// panicking so a caller can report it the same way as any other
// command dispatch failure.
func CaptureOutput(fn func()) ([]string, error) {
	real := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	os.Stdout = w
	defer func() { os.Stdout = real }()

	var buf bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, cerr := io.Copy(&buf, r)
		copyDone <- cerr
	}()

	fn()

	if err := w.Close(); err != nil {
		return nil, err
	}
	if err := <-copyDone; err != nil {
		return nil, err
	}

	var lines []string
	scanner := bufio.NewScanner(&buf)
	// A single captured line can legitimately exceed bufio.Scanner's
	// own 64 KiB default buffer, "show running-config" on a device
	// with a very large configuration for instance, so the buffer is
	// grown well past that default rather than silently truncating,
	// or erroring out on, an unusually long line.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, nil
}
