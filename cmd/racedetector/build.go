// build.go implements the 'racedetector build' command.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kolkov/racedetector/cmd/racedetector/instrument"
	"github.com/kolkov/racedetector/cmd/racedetector/runtime"
)

// buildCommand implements the 'racedetector build' command.
//
// This command instruments Go source files and builds them with race detection.
// It acts as a drop-in replacement for 'go build', supporting all standard flags.
//
// Flow:
//  1. Parse arguments (source files + go build flags)
//  2. Create temporary workspace
//  3. Instrument source files (insert race detection calls)
//  4. Setup runtime linking (go.mod overlay)
//  5. Call 'go build' with instrumented code
//  6. Cleanup temporary files
//
// Example:
//
//	racedetector build main.go
//	racedetector build -o myapp main.go helper.go
//	racedetector build -ldflags="-s -w" .
func buildCommand(args []string) {
	// Parse arguments
	config, err := parseBuildArgs(args)
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

	// Instrument source files
	if err := instrumentSources(config, workspace); err != nil {
		fmt.Fprintf(os.Stderr, "Error instrumenting sources: %v\n", err)
		os.Exit(1)
	}

	// Setup runtime linking
	if err := workspace.setupRuntimeLinking(); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up runtime: %v\n", err)
		os.Exit(1)
	}

	// Build instrumented code
	if err := workspace.build(config); err != nil {
		fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
		os.Exit(1)
	}

	// Success!
	if config.outputFile != "" {
		fmt.Printf("Built successfully: %s\n", config.outputFile)
	}
}

// buildConfig holds configuration for the build command.
type buildConfig struct {
	// Source files to instrument and build
	sourceFiles []string

	// Output binary name (from -o flag)
	outputFile string

	// Additional go build flags
	buildFlags []string

	// Working directory for build
	workDir string

	// Verbose output flag (-v)
	verbose bool
}

// parseBuildArgs parses command-line arguments for 'racedetector build'.
//
// It separates:
//   - Source files (.go files or directories)
//   - Output file (-o flag)
//   - Go build flags (everything else)
//
// Returns buildConfig with parsed arguments.
func parseBuildArgs(args []string) (*buildConfig, error) {
	config := &buildConfig{
		sourceFiles: []string{},
		buildFlags:  []string{},
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	config.workDir = cwd

	// Parse arguments
	expectingValue := false
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// If previous flag expects a value, this is it (even if it starts with -)
		// Example: -ldflags "-s -w"
		if expectingValue {
			config.buildFlags = append(config.buildFlags, arg)
			expectingValue = false
			continue
		}

		// Handle -o flag (output file)
		if arg == "-o" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("-o flag requires an argument")
			}
			i++
			config.outputFile = args[i]
			continue
		}

		// Handle -o=file format
		if strings.HasPrefix(arg, "-o=") {
			config.outputFile = strings.TrimPrefix(arg, "-o=")
			continue
		}

		// Handle -v flag (verbose output)
		if arg == "-v" {
			config.verbose = true
			continue
		}

		// Handle flags (starts with -)
		if strings.HasPrefix(arg, "-") {
			// It's a build flag - pass through to go build
			config.buildFlags = append(config.buildFlags, arg)

			// Check if this flag expects a value (next arg will be consumed)
			expectingValue = needsValue(arg)
			continue
		}

		// No dash prefix - it's a source file
		// Check if it's a .go file or directory
		if strings.HasSuffix(arg, ".go") || arg == "." || arg == ".." {
			config.sourceFiles = append(config.sourceFiles, arg)
		} else {
			// Could be a package path or directory
			config.sourceFiles = append(config.sourceFiles, arg)
		}
	}

	// Default: build current directory if no sources specified
	if len(config.sourceFiles) == 0 {
		config.sourceFiles = []string{"."}
	}

	return config, nil
}

// needsValue returns true if the flag expects a following value.
func needsValue(flag string) bool {
	// Flags that take values
	valueFlags := []string{
		"-ldflags", "-gcflags", "-asmflags", "-gccgoflags",
		"-tags", "-installsuffix", "-buildmode", "-mod",
		"-modfile", "-overlay", "-pkgdir", "-toolexec",
	}

	for _, vf := range valueFlags {
		// Already has = format (e.g., -ldflags=-s)
		if strings.HasPrefix(flag, vf+"=") {
			return false
		}
		// Exact match - needs next arg
		if flag == vf {
			return true
		}
	}

	return false
}

// workspace represents a temporary workspace for instrumented code.
type workspace struct {
	// Root directory of workspace
	dir string

	// Source directory (where instrumented .go files go)
	srcDir string

	// Original source directory (where original .go files come from)
	// Used to find original go.mod for replace directives
	originalSourceDir string
}

// createWorkspace creates a temporary workspace for building instrumented code.
func createWorkspace() (*workspace, error) {
	// Create temp directory
	dir, err := os.MkdirTemp("", "racedetector-build-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Create src subdirectory
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		_ = os.RemoveAll(dir) // Cleanup on error, ignore removal errors
		return nil, fmt.Errorf("failed to create src directory: %w", err)
	}

	return &workspace{
		dir:    dir,
		srcDir: srcDir,
	}, nil
}

// cleanup removes the temporary workspace.
func (w *workspace) cleanup() {
	if w.dir != "" {
		_ = os.RemoveAll(w.dir) // Best effort cleanup, ignore errors
	}
}

// setupRuntimeLinking creates go.mod overlay for runtime linking.
func (w *workspace) setupRuntimeLinking() error {
	// Create go.mod overlay in workspace
	// Pass original source dir to copy replace directives from original go.mod
	overlayPath, err := runtime.ModFileOverlay(w.dir, w.originalSourceDir)
	if err != nil {
		return fmt.Errorf("failed to create go.mod overlay: %w", err)
	}

	// If overlay was created, rename it to go.mod and tidy
	if overlayPath != "" {
		goModPath := filepath.Join(w.dir, "go.mod")
		if err := os.Rename(overlayPath, goModPath); err != nil {
			return fmt.Errorf("failed to setup go.mod: %w", err)
		}

		// Run go mod tidy to update dependencies
		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = w.dir // go.mod is in workspace root, not src/
		tidyCmd.Stdout = os.Stdout
		tidyCmd.Stderr = os.Stderr
		if err := tidyCmd.Run(); err != nil {
			return fmt.Errorf("failed to tidy go.mod: %w", err)
		}
	}

	return nil
}

// build runs 'go build' on the instrumented code in the workspace.
func (w *workspace) build(config *buildConfig) error {
	// Prepare go build command
	args := []string{"build"}

	// Add output file if specified
	if config.outputFile != "" {
		// Make output path absolute
		outputPath := config.outputFile
		if !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(config.workDir, outputPath)
		}
		args = append(args, "-o", outputPath)
	}

	// Add user-specified build flags
	args = append(args, config.buildFlags...)

	// Add runtime build flags
	runtimeFlags := runtime.BuildFlags()
	args = append(args, runtimeFlags...)

	// Build from workspace src directory
	args = append(args, ".")

	// Run go build
	cmd := exec.Command("go", args...)
	cmd.Dir = w.srcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// instrumentSources instruments all source files and writes them to workspace.
func instrumentSources(config *buildConfig, workspace *workspace) error {
	// Collect all .go files to instrument
	goFiles, err := collectGoFiles(config.sourceFiles, config.workDir)
	if err != nil {
		return fmt.Errorf("failed to collect source files: %w", err)
	}

	if len(goFiles) == 0 {
		return fmt.Errorf("no Go source files found")
	}

	// Store original source directory for go.mod replace directive handling
	// Use the first source file's directory or workDir
	if len(goFiles) > 0 {
		workspace.originalSourceDir = filepath.Dir(goFiles[0])
	} else {
		workspace.originalSourceDir = config.workDir
	}

	// v0.8.0: Get escape analysis from Go compiler for more accurate detection
	// This allows us to skip instrumentation of stack-local variables that don't escape to heap.
	var escapeInfo *instrument.EscapeInfo
	escapeInfo, err = instrument.GetEscapeInfoForFiles(goFiles)
	if err != nil {
		// Escape analysis is optional - continue without it if it fails
		// This may happen on syntax errors or incomplete packages
		if config.verbose {
			fmt.Printf("Warning: escape analysis unavailable: %v\n", err)
		}
		escapeInfo = nil
	} else if config.verbose && escapeInfo != nil {
		fmt.Printf("Escape analysis: found %d escaping locations\n", len(escapeInfo.Escapes)/2) // /2 because we store both line:col and line
	}

	// Instrument each file
	for _, srcPath := range goFiles {
		// v0.8.0: Instrument with escape analysis info
		opts := instrument.InstrumentOptions{
			EscapeInfo: escapeInfo,
			Filename:   filepath.Base(srcPath),
		}
		result, err := instrument.InstrumentFileWithOptions(srcPath, nil, opts)
		if err != nil {
			return fmt.Errorf("failed to instrument %s: %w", srcPath, err)
		}

		// Determine output path in workspace
		// Use just the filename (flatten directory structure for simplicity)
		outPath := filepath.Join(workspace.srcDir, filepath.Base(srcPath))

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
			fmt.Printf("Skipped (CGO): %s\n", filepath.Base(srcPath))
			continue
		}

		// Write instrumented code to workspace
		if err := os.WriteFile(outPath, []byte(result.Code), 0644); err != nil {
			return fmt.Errorf("failed to write instrumented file %s: %w", outPath, err)
		}

		// Print instrumentation info
		fmt.Printf("Instrumented: %s\n", filepath.Base(srcPath))

		// If verbose, print statistics
		if config.verbose {
			stats := result.Stats
			fmt.Printf("  - %d writes, %d reads instrumented\n", stats.WritesInstrumented, stats.ReadsInstrumented)
			if stats.TotalSkipped() > 0 {
				fmt.Printf("  - %d items skipped (%d constants, %d built-ins, %d literals, %d blanks",
					stats.TotalSkipped(),
					stats.ConstantsSkipped,
					stats.BuiltinsSkipped,
					stats.LiteralsSkipped,
					stats.BlanksSkipped,
				)
				if stats.EscapeSkipped > 0 {
					fmt.Printf(", %d stack-local via escape analysis", stats.EscapeSkipped)
				}
				fmt.Println(")")
			}
		}
	}

	return nil
}

// collectGoFiles finds all .go files from the given sources.
//
// Sources can be:
//   - .go files directly
//   - directories (scans for .go files)
//   - "." for current directory
func collectGoFiles(sources []string, workDir string) ([]string, error) {
	var goFiles []string

	for _, src := range sources {
		// Make path absolute
		srcPath := src
		if !filepath.IsAbs(srcPath) {
			srcPath = filepath.Join(workDir, src)
		}

		// Check if it's a file or directory
		info, err := os.Stat(srcPath)
		if err != nil {
			return nil, fmt.Errorf("cannot access %s: %w", src, err)
		}

		if info.IsDir() {
			// Scan directory for .go files
			entries, err := os.ReadDir(srcPath)
			if err != nil {
				return nil, fmt.Errorf("cannot read directory %s: %w", srcPath, err)
			}

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				name := entry.Name()
				// Include only .go files (exclude _test.go for build)
				if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
					fullPath := filepath.Join(srcPath, name)
					goFiles = append(goFiles, fullPath)
				}
			}
		} else {
			// It's a file - check if it's a .go file
			if strings.HasSuffix(srcPath, ".go") {
				goFiles = append(goFiles, srcPath)
			}
		}
	}

	return goFiles, nil
}
