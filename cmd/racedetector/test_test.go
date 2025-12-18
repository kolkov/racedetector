// test_test.go implements tests for the 'racedetector test' command.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseTestArgs tests the parseTestArgs function.
func TestParseTestArgs(t *testing.T) {
	// Save and restore working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	tests := []struct {
		name            string
		args            []string
		wantPackages    []string
		wantFlags       []string
		wantVerbose     bool
		wantCompileOnly bool
		wantOutputFile  string
		wantErr         bool
	}{
		{
			name:         "no args - default to current dir",
			args:         []string{},
			wantPackages: []string{"."},
			wantFlags:    []string{},
			wantVerbose:  false,
		},
		{
			name:         "single package",
			args:         []string{"./..."},
			wantPackages: []string{"./..."},
			wantFlags:    []string{},
			wantVerbose:  false,
		},
		{
			name:         "verbose flag",
			args:         []string{"-v", "./..."},
			wantPackages: []string{"./..."},
			wantFlags:    []string{"-v"},
			wantVerbose:  true,
		},
		{
			name:         "run flag with value",
			args:         []string{"-run", "TestFoo", "./pkg/..."},
			wantPackages: []string{"./pkg/..."},
			wantFlags:    []string{"-run", "TestFoo"},
			wantVerbose:  false,
		},
		{
			name:         "run flag with equals",
			args:         []string{"-run=TestBar", "./..."},
			wantPackages: []string{"./..."},
			wantFlags:    []string{"-run=TestBar"},
			wantVerbose:  false,
		},
		{
			name:         "multiple flags",
			args:         []string{"-v", "-cover", "-timeout=30s", "./internal/..."},
			wantPackages: []string{"./internal/..."},
			wantFlags:    []string{"-v", "-cover", "-timeout=30s"},
			wantVerbose:  true,
		},
		{
			name:         "coverage profile",
			args:         []string{"-coverprofile", "coverage.out", "./..."},
			wantPackages: []string{"./..."},
			wantFlags:    []string{"-coverprofile", "coverage.out"},
			wantVerbose:  false,
		},
		{
			name:         "benchmark flags",
			args:         []string{"-bench", ".", "-benchmem", "./..."},
			wantPackages: []string{"./..."},
			wantFlags:    []string{"-bench", ".", "-benchmem"},
			wantVerbose:  false,
		},
		{
			name:         "multiple packages",
			args:         []string{"./pkg1", "./pkg2"},
			wantPackages: []string{"./pkg1", "./pkg2"},
			wantFlags:    []string{},
			wantVerbose:  false,
		},
		{
			name:            "compile only flag",
			args:            []string{"-c", "./..."},
			wantPackages:    []string{"./..."},
			wantFlags:       []string{"-c"},
			wantVerbose:     false,
			wantCompileOnly: true,
		},
		{
			name:            "compile with output flag",
			args:            []string{"-c", "-o", "mytest", "./..."},
			wantPackages:    []string{"./..."},
			wantFlags:       []string{"-c"},
			wantVerbose:     false,
			wantCompileOnly: true,
			wantOutputFile:  "mytest",
		},
		{
			name:            "output flag with equals",
			args:            []string{"-c", "-o=mytest.exe", "./pkg"},
			wantPackages:    []string{"./pkg"},
			wantFlags:       []string{"-c"},
			wantVerbose:     false,
			wantCompileOnly: true,
			wantOutputFile:  "mytest.exe",
		},
		{
			name:            "verbose compile with output",
			args:            []string{"-v", "-c", "-o", "test.bin", "./..."},
			wantPackages:    []string{"./..."},
			wantFlags:       []string{"-v", "-c"},
			wantVerbose:     true,
			wantCompileOnly: true,
			wantOutputFile:  "test.bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := parseTestArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTestArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			// Check packages
			if len(config.packages) != len(tt.wantPackages) {
				t.Errorf("packages = %v, want %v", config.packages, tt.wantPackages)
			} else {
				for i, pkg := range config.packages {
					if pkg != tt.wantPackages[i] {
						t.Errorf("packages[%d] = %q, want %q", i, pkg, tt.wantPackages[i])
					}
				}
			}

			// Check flags
			if len(config.testFlags) != len(tt.wantFlags) {
				t.Errorf("testFlags = %v, want %v", config.testFlags, tt.wantFlags)
			} else {
				for i, flag := range config.testFlags {
					if flag != tt.wantFlags[i] {
						t.Errorf("testFlags[%d] = %q, want %q", i, flag, tt.wantFlags[i])
					}
				}
			}

			// Check verbose
			if config.verbose != tt.wantVerbose {
				t.Errorf("verbose = %v, want %v", config.verbose, tt.wantVerbose)
			}

			// Check compileOnly
			if config.compileOnly != tt.wantCompileOnly {
				t.Errorf("compileOnly = %v, want %v", config.compileOnly, tt.wantCompileOnly)
			}

			// Check outputFile
			if config.outputFile != tt.wantOutputFile {
				t.Errorf("outputFile = %q, want %q", config.outputFile, tt.wantOutputFile)
			}
		})
	}
}

// TestTestFlagNeedsValue tests the testFlagNeedsValue function.
func TestTestFlagNeedsValue(t *testing.T) {
	tests := []struct {
		flag string
		want bool
	}{
		{"-run", true},
		{"-run=TestFoo", false},
		{"-bench", true},
		{"-bench=.", false},
		{"-timeout", true},
		{"-timeout=30s", false},
		{"-coverprofile", true},
		{"-coverprofile=c.out", false},
		{"-v", false},
		{"-cover", false},
		{"-benchmem", false},
		{"-count", true},
		{"-cpu", true},
		{"-parallel", true},
		{"-ldflags", true},
		{"-tags", true},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got := testFlagNeedsValue(tt.flag)
			if got != tt.want {
				t.Errorf("testFlagNeedsValue(%q) = %v, want %v", tt.flag, got, tt.want)
			}
		})
	}
}

// TestResolvePackagePatterns tests the resolvePackagePatterns function.
func TestResolvePackagePatterns(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "racedetector-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create subdirectories with Go files
	pkg1 := filepath.Join(tmpDir, "pkg1")
	pkg2 := filepath.Join(tmpDir, "pkg2")
	subpkg := filepath.Join(tmpDir, "pkg1", "sub")

	for _, dir := range []string{pkg1, pkg2, subpkg} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		// Create a .go file in each
		goFile := filepath.Join(dir, "test.go")
		if err := os.WriteFile(goFile, []byte("package test\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		patterns []string
		wantLen  int
	}{
		{
			name:     "single directory",
			patterns: []string{pkg1},
			wantLen:  1,
		},
		{
			name:     "recursive pattern",
			patterns: []string{tmpDir + "/..."},
			wantLen:  3, // tmpDir itself may or may not have .go files, but pkg1, pkg2, subpkg do
		},
		{
			name:     "current directory",
			patterns: []string{"."},
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs, err := resolvePackagePatterns(tt.patterns, tmpDir)
			if err != nil {
				t.Errorf("resolvePackagePatterns() error = %v", err)
				return
			}
			if len(dirs) < 1 {
				t.Errorf("resolvePackagePatterns() returned no directories")
			}
		})
	}
}

// TestHasGoFiles tests the hasGoFiles function.
func TestHasGoFiles(t *testing.T) {
	// Create temp directory with Go file
	tmpDirWithGo, err := os.MkdirTemp("", "racedetector-test-go-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDirWithGo)

	goFile := filepath.Join(tmpDirWithGo, "test.go")
	if err := os.WriteFile(goFile, []byte("package test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create temp directory without Go file
	tmpDirNoGo, err := os.MkdirTemp("", "racedetector-test-nogo-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDirNoGo)

	txtFile := filepath.Join(tmpDirNoGo, "readme.txt")
	if err := os.WriteFile(txtFile, []byte("not go\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{
			name: "directory with go files",
			dir:  tmpDirWithGo,
			want: true,
		},
		{
			name: "directory without go files",
			dir:  tmpDirNoGo,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hasGoFiles(tt.dir)
			if err != nil {
				t.Errorf("hasGoFiles() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("hasGoFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCollectTestGoFiles tests the collectTestGoFiles function.
func TestCollectTestGoFiles(t *testing.T) {
	// Create temp directory with various files
	tmpDir, err := os.MkdirTemp("", "racedetector-test-collect-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create files
	files := map[string]string{
		"main.go":      "package main\n",
		"main_test.go": "package main\n",
		"helper.go":    "package main\n",
		"readme.txt":   "not go\n",
	}
	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	goFiles, err := collectTestGoFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Should have 3 .go files (main.go, main_test.go, helper.go)
	if len(goFiles) != 3 {
		t.Errorf("collectTestGoFiles() returned %d files, want 3", len(goFiles))
	}

	// Verify all are .go files
	for _, f := range goFiles {
		if filepath.Ext(f) != ".go" {
			t.Errorf("collectTestGoFiles() returned non-.go file: %s", f)
		}
	}
}
