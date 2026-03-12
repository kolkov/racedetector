// test.go implements the 'racedetector test' command.
package main

import (
	"encoding/json"
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

	// Output file for -c -o (compile test binary without running)
	outputFile string

	// Compile only flag (-c)
	compileOnly bool
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

	// Runtime linking is handled via -modfile flag (no setupRuntimeLinking needed)

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

		// Handle -c flag (compile only, don't run)
		if arg == "-c" {
			config.compileOnly = true
			config.testFlags = append(config.testFlags, arg)
			continue
		}

		// Handle -o flag (output file) - need to track for later copying
		if arg == "-o" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-o flag requires an argument")
			}
			i++
			config.outputFile = args[i]
			// Don't add to testFlags yet - we'll handle output path in runTests
			continue
		}

		// Handle -o=file format
		if strings.HasPrefix(arg, "-o=") {
			config.outputFile = strings.TrimPrefix(arg, "-o=")
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

// instrumentTestSources instruments source files and creates a Go overlay.
//
// Instead of copying files to a temp workspace (which breaks internal/ imports
// and //go:embed), this creates a Go overlay JSON that maps original files to
// their instrumented replacements. The Go toolchain reads all other files from
// the original module tree, so internal packages, embedded assets, and all
// imports resolve correctly.
func instrumentTestSources(config *testConfig, workspace *workspace) error {
	// Resolve package patterns to actual directories
	dirs, err := resolvePackagePatterns(config.packages, config.workDir)
	if err != nil {
		return fmt.Errorf("failed to resolve packages: %w", err)
	}

	if len(dirs) == 0 {
		return fmt.Errorf("no packages found matching patterns: %v", config.packages)
	}

	workspace.originalSourceDir = config.workDir

	// Create directory for instrumented files
	instrDir := filepath.Join(workspace.dir, "instr")
	if err := os.MkdirAll(instrDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Build overlay: original absolute path → instrumented absolute path
	replace := make(map[string]string)

	for _, dir := range dirs {
		goFiles, err := collectTestGoFiles(dir)
		if err != nil {
			return fmt.Errorf("failed to collect files from %s: %w", dir, err)
		}

		for _, srcPath := range goFiles {
			result, err := instrument.InstrumentFile(srcPath, nil)
			if err != nil {
				return fmt.Errorf("failed to instrument %s: %w", srcPath, err)
			}

			relPath, relErr := filepath.Rel(config.workDir, srcPath)
			if relErr != nil {
				relPath = filepath.Base(srcPath)
			}

			// CGO files: skip instrumentation, no overlay entry needed
			if result.Stats.CGOSkipped {
				fmt.Printf("Skipped (CGO): %s\n", relPath)
				continue
			}

			// Write instrumented file to temp dir, preserving relative path
			outPath := filepath.Join(instrDir, relPath)
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return fmt.Errorf("failed to create directory for %s: %w", outPath, err)
			}
			if err := os.WriteFile(outPath, []byte(result.Code), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", outPath, err)
			}

			// Overlay requires absolute paths
			absSrc, _ := filepath.Abs(srcPath)
			absOut, _ := filepath.Abs(outPath)
			replace[absSrc] = absOut

			fmt.Printf("Instrumented: %s\n", relPath)
			if config.verbose {
				stats := result.Stats
				if stats.Total() > 0 {
					fmt.Printf("  - %d writes, %d reads instrumented\n",
						stats.WritesInstrumented, stats.ReadsInstrumented)
				}
			}
		}
	}

	// Create modified go.mod with racedetector dependency (for -modfile flag).
	// We use -modfile instead of overlaying go.mod because the overlaid go.mod
	// would require "go mod tidy" (writes to original files through overlay).
	// With -modfile, Go reads/writes race.mod and race.sum in the temp dir,
	// leaving the user's go.mod/go.sum untouched.
	goModSrc := filepath.Join(config.workDir, "go.mod")
	if data, err := os.ReadFile(goModSrc); err == nil {
		version := runtime.GetVersion()
		modContent := string(data) + fmt.Sprintf("\nrequire github.com/kolkov/racedetector %s\n", version)

		raceModPath := filepath.Join(workspace.dir, "race.mod")
		if err := os.WriteFile(raceModPath, []byte(modContent), 0644); err != nil {
			return fmt.Errorf("failed to write race.mod: %w", err)
		}
		workspace.modfilePath = raceModPath

		// Copy go.sum → race.sum (Go derives sum path from modfile name)
		goSumSrc := filepath.Join(config.workDir, "go.sum")
		raceSumPath := filepath.Join(workspace.dir, "race.sum")
		if sumData, sumErr := os.ReadFile(goSumSrc); sumErr == nil {
			_ = os.WriteFile(raceSumPath, sumData, 0644)
		}
	}

	// Write overlay JSON (must be after all entries are added to replace map)
	type overlayJSON struct {
		Replace map[string]string `json:"Replace"`
	}
	overlayData, err := json.Marshal(overlayJSON{Replace: replace})
	if err != nil {
		return fmt.Errorf("failed to marshal overlay: %w", err)
	}
	workspace.overlayPath = filepath.Join(workspace.dir, "overlay.json")
	if err := os.WriteFile(workspace.overlayPath, overlayData, 0644); err != nil {
		return fmt.Errorf("failed to write overlay: %w", err)
	}

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

// runTests executes 'go test' with the overlay from the original module directory.
//
// Instead of running from a temp workspace, this runs from the original project
// directory with -overlay and -modfile flags. This ensures all imports (including
// internal/ packages) and //go:embed directives resolve correctly.
func runTests(workspace *workspace, config *testConfig) int {
	args := []string{"test"}

	// Use overlay for instrumented files
	if workspace.overlayPath != "" {
		args = append(args, "-overlay="+workspace.overlayPath)
	}

	// Use modified go.mod with racedetector dependency.
	// -mod=mod allows Go to update race.sum for new dependencies.
	// GOWORK=off disables workspace mode (incompatible with -modfile).
	// All writes go to race.mod/race.sum in the temp dir, not user files.
	if workspace.modfilePath != "" {
		args = append(args, "-modfile="+workspace.modfilePath)
		args = append(args, "-mod=mod")
	}

	// Add test flags
	args = append(args, config.testFlags...)

	// Add runtime build flags
	runtimeFlags := runtime.BuildFlags()
	args = append(args, runtimeFlags...)

	// Handle -o flag (works directly since we run from original workDir)
	if config.outputFile != "" {
		outputPath := config.outputFile
		if !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(config.workDir, outputPath)
		}
		args = append(args, "-o", outputPath)
	}

	// Use original package patterns (resolved from the real module tree)
	args = append(args, config.packages...)

	// Run go test from the ORIGINAL project directory
	cmd := exec.Command("go", args...)
	cmd.Dir = config.workDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// GOWORK=off: disable workspace mode which is incompatible with -modfile.
	// GONOSUMCHECK: skip checksum verification for racedetector module.
	cmd.Env = append(os.Environ(),
		"GOWORK=off",
		"GONOSUMCHECK=github.com/kolkov/racedetector",
	)

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "Error executing tests: %v\n", err)
		return 1
	}

	return 0
}
