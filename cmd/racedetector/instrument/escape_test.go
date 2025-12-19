// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

package instrument

import (
	"testing"
)

func TestParseEscapeOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLocs []string // Expected escape locations
		wantVars map[string]string
	}{
		{
			name: "moved to heap",
			input: `# command-line-arguments
./main.go:10:5: moved to heap: x
./main.go:15:2: y does not escape`,
			wantLocs: []string{"main.go:10:5", "main.go:10"},
			wantVars: map[string]string{"main.go:10:5": "x", "main.go:10": "x"},
		},
		{
			name: "escapes to heap",
			input: `./main.go:20:6: z escapes to heap
./main.go:25:3: w does not escape`,
			wantLocs: []string{"main.go:20:6", "main.go:20"},
			wantVars: map[string]string{"main.go:20:6": "z", "main.go:20": "z"},
		},
		{
			name:     "new escapes",
			input:    `./main.go:30:13: new(int) escapes to heap`,
			wantLocs: []string{"main.go:30:13", "main.go:30"},
			wantVars: map[string]string{}, // no var name for new()
		},
		{
			name:     "make escapes",
			input:    `./main.go:35:10: make([]int, 10) escapes to heap`,
			wantLocs: []string{"main.go:35:10", "main.go:35"},
			wantVars: map[string]string{},
		},
		{
			name:     "func literal escapes",
			input:    `./main.go:40:8: func literal escapes to heap`,
			wantLocs: []string{"main.go:40:8", "main.go:40"},
			wantVars: map[string]string{},
		},
		{
			name:     "address-of escapes",
			input:    `./main.go:45:5: &data escapes to heap`,
			wantLocs: []string{"main.go:45:5", "main.go:45"},
			wantVars: map[string]string{"main.go:45:5": "data", "main.go:45": "data"},
		},
		{
			name: "multiple escapes",
			input: `./pkg/foo.go:10:2: moved to heap: a
./pkg/foo.go:15:6: b escapes to heap
./pkg/bar.go:20:3: c does not escape
./pkg/bar.go:25:8: new(string) escapes to heap`,
			// Note: normalizeFilePath now extracts basename for robust matching
			wantLocs: []string{
				"foo.go:10:2", "foo.go:10",
				"foo.go:15:6", "foo.go:15",
				"bar.go:25:8", "bar.go:25",
			},
			wantVars: map[string]string{
				"foo.go:10:2": "a", "foo.go:10": "a",
				"foo.go:15:6": "b", "foo.go:15": "b",
			},
		},
		{
			name:     "no escapes",
			input:    `./main.go:10:5: x does not escape`,
			wantLocs: []string{},
			wantVars: map[string]string{},
		},
		{
			name:  "windows paths",
			input: `C:\Users\test\main.go:10:5: moved to heap: x`,
			// Note: normalizeFilePath extracts basename, so full Windows path becomes just "main.go"
			wantLocs: []string{"main.go:10:5", "main.go:10"},
			wantVars: map[string]string{
				"main.go:10:5": "x",
				"main.go:10":   "x",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseEscapeOutput([]byte(tt.input))
			if err != nil {
				t.Fatalf("ParseEscapeOutput() error = %v", err)
			}

			// Check expected locations are present
			for _, loc := range tt.wantLocs {
				if !info.Escapes[loc] {
					t.Errorf("Expected escape at %s, but not found", loc)
				}
			}

			// Check no unexpected locations
			for loc := range info.Escapes {
				found := false
				for _, want := range tt.wantLocs {
					if loc == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Unexpected escape at %s", loc)
				}
			}

			// Check variable names
			for loc, wantName := range tt.wantVars {
				if gotName := info.VarNames[loc]; gotName != wantName {
					t.Errorf("VarNames[%s] = %q, want %q", loc, gotName, wantName)
				}
			}
		})
	}
}

func TestDoesEscape(t *testing.T) {
	info := &EscapeInfo{
		Escapes: map[string]bool{
			"main.go:10:5": true,
			"main.go:10":   true,
			"foo.go:20":    true,
		},
		VarNames: map[string]string{},
	}

	tests := []struct {
		file string
		line int
		col  int
		want bool
	}{
		{"main.go", 10, 5, true},
		{"main.go", 10, 0, true},  // No column, should still match
		{"main.go", 10, 3, true},  // Different column, but line matches (conservative)
		{"main.go", 11, 5, false}, // Wrong line
		{"foo.go", 20, 0, true},
		{"./foo.go", 20, 0, true}, // With ./ prefix
		{"bar.go", 30, 0, false},  // Not in map
	}

	for _, tt := range tests {
		got := info.DoesEscape(tt.file, tt.line, tt.col)
		if got != tt.want {
			t.Errorf("DoesEscape(%q, %d, %d) = %v, want %v",
				tt.file, tt.line, tt.col, got, tt.want)
		}
	}
}
