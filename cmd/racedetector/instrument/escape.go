// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

package instrument

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// EscapeInfo holds escape analysis results from Go compiler.
// Maps "file.go:line:col" or "file.go:line" to escape status.
type EscapeInfo struct {
	// Escapes contains locations where variables escape to heap.
	// Key format: "file.go:10:5" or "file.go:10"
	Escapes map[string]bool

	// VarNames maps location to variable name that escapes.
	// Key format: "file.go:10:5", Value: "x"
	VarNames map[string]string
}

// escapePatterns matches Go compiler escape analysis output.
// Examples:
//
//	./main.go:10:5: moved to heap: x
//	./main.go:15:2: x escapes to heap
//	./main.go:20:13: new(int) escapes to heap
//	./main.go:25:6: func literal escapes to heap
var escapePatterns = []*regexp.Regexp{
	// "moved to heap: varname"
	regexp.MustCompile(`^(.+):(\d+):(\d+): moved to heap: (\w+)`),
	// "varname escapes to heap"
	regexp.MustCompile(`^(.+):(\d+):(\d+): (\w+) escapes to heap`),
	// "new(Type) escapes to heap"
	regexp.MustCompile(`^(.+):(\d+):(\d+): new\(.+\) escapes to heap`),
	// "make(...) escapes to heap"
	regexp.MustCompile(`^(.+):(\d+):(\d+): make\(.+\) escapes to heap`),
	// "func literal escapes to heap"
	regexp.MustCompile(`^(.+):(\d+):(\d+): func literal escapes to heap`),
	// "&varname escapes to heap"
	regexp.MustCompile(`^(.+):(\d+):(\d+): &(\w+) escapes to heap`),
}

// doesNotEscapePattern matches "does not escape" messages.
var doesNotEscapePattern = regexp.MustCompile(`^(.+):(\d+):(\d+): .+ does not escape`)

// GetEscapeInfo runs Go compiler with -gcflags="-m" and parses escape analysis output.
// Returns EscapeInfo with locations of escaping variables.
//
// Example usage:
//
//	info, err := GetEscapeInfo("./...")
//	if err != nil {
//	    return err
//	}
//	if info.Escapes["main.go:10:5"] {
//	    // Variable at this location escapes - instrument it
//	}
func GetEscapeInfo(pkgPath string) (*EscapeInfo, error) {
	// Run go build with escape analysis output
	cmd := exec.Command("go", "build", "-gcflags=-m", pkgPath)
	output, err := cmd.CombinedOutput()

	// go build returns non-zero if there are compile errors,
	// but we still get escape analysis output for valid code
	if err != nil {
		// Check if it's a real error or just escape output
		if !strings.Contains(string(output), "escapes to heap") &&
			!strings.Contains(string(output), "does not escape") &&
			!strings.Contains(string(output), "moved to heap") {
			return nil, err
		}
	}

	return ParseEscapeOutput(output)
}

// GetEscapeInfoForFiles runs escape analysis on specific files.
func GetEscapeInfoForFiles(files []string) (*EscapeInfo, error) {
	args := append([]string{"build", "-gcflags=-m"}, files...)
	cmd := exec.Command("go", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if !strings.Contains(string(output), "escapes to heap") &&
			!strings.Contains(string(output), "does not escape") &&
			!strings.Contains(string(output), "moved to heap") {
			return nil, err
		}
	}

	return ParseEscapeOutput(output)
}

// ParseEscapeOutput parses Go compiler escape analysis output.
// This is exported for testing purposes.
func ParseEscapeOutput(output []byte) (*EscapeInfo, error) {
	info := &EscapeInfo{
		Escapes:  make(map[string]bool),
		VarNames: make(map[string]string),
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		// Skip non-escape lines
		if !strings.Contains(line, "escape") && !strings.Contains(line, "moved to heap") {
			continue
		}

		// Check if it's a "does not escape" message - skip these
		if doesNotEscapePattern.MatchString(line) {
			continue
		}

		// Try each escape pattern
		for _, pattern := range escapePatterns {
			matches := pattern.FindStringSubmatch(line)
			if matches == nil {
				continue
			}

			// Extract location: file:line:col
			file := normalizeFilePath(matches[1])
			lineNum := matches[2]
			col := matches[3]

			// Build location key
			loc := file + ":" + lineNum + ":" + col
			locShort := file + ":" + lineNum // Also store without column

			info.Escapes[loc] = true
			info.Escapes[locShort] = true

			// Extract variable name if present
			if len(matches) > 4 && matches[4] != "" {
				info.VarNames[loc] = matches[4]
				info.VarNames[locShort] = matches[4]
			}

			break
		}
	}

	return info, scanner.Err()
}

// normalizeFilePath normalizes file path for consistent matching.
// Removes ./ prefix, converts to forward slashes, and extracts basename.
// Using basename ensures consistent matching between escape analysis output
// (which uses relative paths) and AST positions (which use absolute paths).
func normalizeFilePath(path string) string {
	// Remove ./ prefix
	path = strings.TrimPrefix(path, "./")
	// Convert backslashes to forward slashes (Windows)
	path = strings.ReplaceAll(path, "\\", "/")

	// Extract basename for more robust matching
	// Escape analysis outputs relative paths (e.g., "examples/foo/main.go")
	// But AST parsing uses absolute paths (e.g., "D:/projects/foo/main.go")
	// Using basename allows matching regardless of path prefix.
	//
	// Note: This may cause false matches if same filename exists in different dirs.
	// For now, this is acceptable as it's conservative (may skip less, not more).
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[idx+1:]
	}
	return path
}

// DoesEscape checks if a variable at given location escapes to heap.
// Location can be "file.go:10:5" or "file.go:10".
func (e *EscapeInfo) DoesEscape(file string, line, col int) bool {
	file = normalizeFilePath(file)

	// Try with column
	if col > 0 {
		loc := fmt.Sprintf("%s:%d:%d", file, line, col)
		if e.Escapes[loc] {
			return true
		}
	}

	// Try without column
	loc := fmt.Sprintf("%s:%d", file, line)
	return e.Escapes[loc]
}

// GetVarName returns the variable name at given location, if known.
func (e *EscapeInfo) GetVarName(file string, line, col int) string {
	file = normalizeFilePath(file)

	if col > 0 {
		loc := fmt.Sprintf("%s:%d:%d", file, line, col)
		if name, ok := e.VarNames[loc]; ok {
			return name
		}
	}

	loc := fmt.Sprintf("%s:%d", file, line)
	return e.VarNames[loc]
}
