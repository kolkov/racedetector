// test.go implements the 'racedetector test' command.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kolkov/racedetector/cmd/racedetector/instrument"
	"github.com/kolkov/racedetector/cmd/racedetector/runtime"
)

// testConfig holds configuration for the test command.
type testConfig struct {
	// Package patterns to test (e.g., "./...", "./internal/...")
	packages []string

	// Test flags to pass to go test (-v, -run, -bench, etc.)
	testFlags []string

	// Working directory
	workDir string

	// Verbose output flag (-v)
	verbose bool
}

// testCommand implements the 'racedetector test' command.
//
// This command instruments Go source files (including test files),
// and runs 'go test' with race detection enabled.
// It acts as a drop-in replacement for 'go test -race'.
//
// Flow:
//  1. Parse arguments (test flags + package patterns)
//  2. Create temporary workspace
//  3. Instrument source files (including _test.go)
//  4. Setup runtime linking (go.mod overlay)
//  5. Call 'go test' with instrumented code
//  6. Forward test output and exit code
//  7. Cleanup temporary files
//
// Example:
//
//	racedetector test ./...
//	racedetector test -v ./internal/...
//	racedetector test -run=TestMyFunction ./pkg/mypackage
//	racedetector test -cover -coverprofile=coverage.out ./...
func testCommand(args []string) {
	// Parse arguments
	config, err := parseTestArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Validate runtime is available
	if err := runtime.ValidateRuntimeAvailable(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Race detector runtime not found\n")
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Fprintf(os.Stderr, "\nPlease ensure the runtime is installed:\n")
		fmt.Fprintf(os.Stderr, "  go get github.com/kolkov/racedetector/internal/race/api\n")
		os.Exit(1)
	}

	// Create temporary workspace
	workspace, err := createWorkspace()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating workspace: %v\n", err)
		os.Exit(1)
	}
	defer workspace.cleanup()

	// Instrument source files (including test files)
	if err := instrumentTestSources(config, workspace); err != nil {
		fmt.Fprintf(os.Stderr, "Error instrumenting sources: %v\n", err)
		os.Exit(1)
	}

	// Setup runtime linking
	if err := workspace.setupRuntimeLinking(); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up runtime: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	exitCode := runTests(workspace, config)
	os.Exit(exitCode)
}

// parseTestArgs parses command-line arguments for 'racedetector test'.
//
// The 'go test' command format is:
//
//	go test [build/test flags] [packages] [test binary flags]
//
// We support:
//
//	racedetector test ./...
//	racedetector test -v ./internal/...
//	racedetector test -run=TestFoo -v ./pkg/...
//	racedetector test -cover -coverprofile=c.out ./...
//
// Returns testConfig with parsed arguments.
func parseTestArgs(args []string) (*testConfig, error) {
	config := &testConfig{
		packages:  []string{},
		testFlags: []string{},
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	config.workDir = cwd

	// Parse arguments
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Handle -v flag specially (we use it too)
		if arg == "-v" {
			config.verbose = true
			config.testFlags = append(config.testFlags, arg)
			continue
		}

		// Handle flags (starts with -)
		if strings.HasPrefix(arg, "-") {
			config.testFlags = append(config.testFlags, arg)

			// Check if this flag expects a value (next arg will be consumed)
			if testFlagNeedsValue(arg) && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				config.testFlags = append(config.testFlags, args[i])
			}
			continue
		}

		// No dash prefix - it's a package pattern
		config.packages = append(config.packages, arg)
	}

	// Default: test current directory if no packages specified
	if len(config.packages) == 0 {
		config.packages = []string{"."}
	}

	return config, nil
}

// testFlagNeedsValue returns true if the test flag expects a following value.
func testFlagNeedsValue(flag string) bool {
	// Already has = format (e.g., -run=TestFoo)
	if strings.Contains(flag, "=") {
		return false
	}

	// Flags that take values (without = format)
	valueFlags := []string{
		"-run", "-bench", "-benchtime", "-blockprofile", "-blockprofilerate",
		"-coverprofile", "-covermode", "-count", "-cpu", "-cpuprofile",
		"-memprofile", "-memprofilerate", "-mutexprofile", "-mutexprofilefraction",
		"-outputdir", "-parallel", "-timeout", "-trace",
		// Build flags that may appear
		"-ldflags", "-gcflags", "-tags", "-mod", "-modfile",
	}

	for _, vf := range valueFlags {
		if flag == vf {
			return true
		}
	}

	return false
}

// instrumentTestSources instruments all source files including test files.
func instrumentTestSources(config *testConfig, workspace *workspace) error {
	// Resolve package patterns to actual directories
	dirs, err := resolvePackagePatterns(config.packages, config.workDir)
	if err != nil {
		return fmt.Errorf("failed to resolve packages: %w", err)
	}

	if len(dirs) == 0 {
		return fmt.Errorf("no packages found matching patterns: %v", config.packages)
	}

	// Store original source directory for go.mod replace directive handling
	workspace.originalSourceDir = config.workDir

	// Collect all .go files (including test files)
	var allGoFiles []string
	for _, dir := range dirs {
		goFiles, err := collectTestGoFiles(dir)
		if err != nil {
			return fmt.Errorf("failed to collect files from %s: %w", dir, err)
		}
		allGoFiles = append(allGoFiles, goFiles...)
	}

	if len(allGoFiles) == 0 {
		return fmt.Errorf("no Go source files found")
	}

	// Instrument each file
	for _, srcPath := range allGoFiles {
		// Instrument the file
		result, err := instrument.InstrumentFile(srcPath, nil)
		if err != nil {
			return fmt.Errorf("failed to instrument %s: %w", srcPath, err)
		}

		// Determine output path in workspace
		// Preserve relative path structure for package resolution
		relPath, err := filepath.Rel(config.workDir, srcPath)
		if err != nil {
			// Fallback to just filename
			relPath = filepath.Base(srcPath)
		}

		outPath := filepath.Join(workspace.srcDir, relPath)

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", outPath, err)
		}

		// Handle CGO files: copy unchanged instead of instrumenting
		if result.Stats.CGOSkipped {
			// Read original file and copy to workspace unchanged
			originalCode, readErr := os.ReadFile(srcPath)
			if readErr != nil {
				return fmt.Errorf("failed to read CGO file %s: %w", srcPath, readErr)
			}
			if err := os.WriteFile(outPath, originalCode, 0644); err != nil {
				return fmt.Errorf("failed to copy CGO file %s: %w", outPath, err)
			}
			fmt.Printf("Skipped (CGO): %s\n", relPath)
			continue
		}

		// Write instrumented code to workspace
		if err := os.WriteFile(outPath, []byte(result.Code), 0644); err != nil {
			return fmt.Errorf("failed to write instrumented file %s: %w", outPath, err)
		}

		// Print instrumentation info
		fmt.Printf("Instrumented: %s\n", relPath)
		if config.verbose {
			stats := result.Stats
			if stats.Total() > 0 {
				fmt.Printf("  - %d writes, %d reads instrumented\n",
					stats.WritesInstrumented, stats.ReadsInstrumented)
			}
		}
	}

	// Copy go.mod to srcDir and add racedetector dependency
	// The replace directive points to ./src, which must be a valid module
	// AND instrumented code imports github.com/kolkov/racedetector/race
	goModSrc := filepath.Join(config.workDir, "go.mod")
	if _, err := os.Stat(goModSrc); err == nil {
		goModDst := filepath.Join(workspace.srcDir, "go.mod")
		data, err := os.ReadFile(goModSrc)
		if err == nil {
			// Append racedetector require to the go.mod
			modContent := string(data)
			modContent += fmt.Sprintf("\nrequire github.com/kolkov/racedetector %s\n", runtime.Version)
			_ = os.WriteFile(goModDst, []byte(modContent), 0644)
		}
	}

	// Also copy go.sum if exists
	goSumSrc := filepath.Join(config.workDir, "go.sum")
	if _, err := os.Stat(goSumSrc); err == nil {
		goSumDst := filepath.Join(workspace.srcDir, "go.sum")
		data, err := os.ReadFile(goSumSrc)
		if err == nil {
			_ = os.WriteFile(goSumDst, data, 0644)
		}
	}

	// Run go mod tidy in srcDir to update go.sum with racedetector dependency
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = workspace.srcDir
	// Ignore errors - tidy may complain about missing imports that we'll resolve later
	_ = tidyCmd.Run()

	return nil
}

// resolvePackagePatterns resolves package patterns like "./..." to directories.
func resolvePackagePatterns(patterns []string, workDir string) ([]string, error) {
	var dirs []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		// Handle "./..." pattern (recursive)
		if strings.HasSuffix(pattern, "/...") || strings.HasSuffix(pattern, "\\...") {
			baseDir := strings.TrimSuffix(strings.TrimSuffix(pattern, "/..."), "\\...")
			if baseDir == "." || baseDir == "" {
				baseDir = workDir
			} else if !filepath.IsAbs(baseDir) {
				baseDir = filepath.Join(workDir, baseDir)
			}

			// Walk directory tree
			err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() {
					return nil
				}
				// Skip hidden directories and vendor
				name := info.Name()
				if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" {
					return filepath.SkipDir
				}
				// Check if directory has .go files
				hasGo, _ := hasGoFiles(path)
				if hasGo && !seen[path] {
					dirs = append(dirs, path)
					seen[path] = true
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to walk %s: %w", baseDir, err)
			}
		} else {
			// Single directory or package
			dir := pattern
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workDir, pattern)
			}

			// Handle "." as current directory
			if pattern == "." {
				dir = workDir
			}

			if !seen[dir] {
				dirs = append(dirs, dir)
				seen[dir] = true
			}
		}
	}

	return dirs, nil
}

// hasGoFiles checks if a directory contains any .go files.
func hasGoFiles(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true, nil
		}
	}

	return false, nil
}

// collectTestGoFiles collects all .go files from a directory (including _test.go).
func collectTestGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var goFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Include all .go files (including _test.go files!)
		if strings.HasSuffix(name, ".go") {
			fullPath := filepath.Join(dir, name)
			goFiles = append(goFiles, fullPath)
		}
	}

	return goFiles, nil
}

// runTests executes 'go test' in the workspace with instrumented code.
func runTests(workspace *workspace, config *testConfig) int {
	// Prepare go test command
	args := []string{"test"}

	// Add test flags
	args = append(args, config.testFlags...)

	// Add runtime build flags
	runtimeFlags := runtime.BuildFlags()
	args = append(args, runtimeFlags...)

	// Test the current package (instrumented sources are in workspace)
	args = append(args, "./...")

	// Run go test
	cmd := exec.Command("go", args...)
	cmd.Dir = workspace.srcDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Check if it's an exit error
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		// Other error (failed to start, etc.)
		fmt.Fprintf(os.Stderr, "Error executing tests: %v\n", err)
		return 1
	}

	return 0
}
