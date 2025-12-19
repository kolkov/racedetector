// toolexec.go implements the 'racedetector toolexec' command.
//
// This command is designed to be used with Go's -toolexec flag:
//
//	go build -toolexec="racedetector toolexec" ./...
//
// When invoked, Go will call racedetector for each tool (compile, link, etc.)
// and racedetector will intercept compile calls to instrument the source files.
//
// IMPORTANT: Toolexec mode requires the racedetector package to be in your
// project's dependencies. Add it first:
//
//	go get github.com/kolkov/racedetector/race
//
// For projects without this dependency, use 'racedetector build' instead
// which automatically handles the runtime linking.
//
// Current Limitations:
//   - Requires racedetector/race package in project dependencies
//   - Does not modify go.mod or importcfg
//   - Standard library is skipped (cannot add external dependencies)
//   - Test files (*_test.go) are passed through unchanged
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kolkov/racedetector/cmd/racedetector/instrument"
)

// toolexecCommand implements the 'racedetector toolexec' command.
//
// Usage:
//
//	go build -toolexec="racedetector toolexec" ./...
//	go test -toolexec="racedetector toolexec" ./...
//
// The command intercepts Go toolchain calls and instruments source files
// before they are compiled. This provides seamless integration with
// standard Go build commands.
//
// Flow:
//  1. Go calls: racedetector toolexec /path/to/compile -o out.a file1.go file2.go
//  2. We detect it's a compile call
//  3. Run escape analysis on source files
//  4. Instrument source files with escape info
//  5. Replace source files in command with instrumented versions
//  6. Execute the real compile command
//  7. Return the same exit code
func toolexecCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "toolexec: no tool specified")
		os.Exit(1)
	}

	toolPath := args[0]
	toolName := filepath.Base(toolPath)
	toolArgs := args[1:]

	// Remove .exe suffix on Windows
	toolName = strings.TrimSuffix(toolName, ".exe")

	// Only intercept compile calls
	if toolName != "compile" {
		// Pass through to the real tool
		exitCode := runTool(toolPath, toolArgs)
		os.Exit(exitCode)
	}

	// Handle compile command - instrument source files
	exitCode := handleCompile(toolPath, toolArgs)
	os.Exit(exitCode)
}

// handleCompile intercepts the compile command and instruments source files.
//
// The compile command looks like:
//
//	compile -o output.a -p package -complete ... file1.go file2.go
//
// We need to:
//  1. Extract .go files from arguments
//  2. Run escape analysis
//  3. Instrument each file
//  4. Replace file paths in arguments with instrumented versions
//  5. Run the real compile
func handleCompile(compilePath string, args []string) int {
	// Parse compile arguments to find source files
	var goFiles []string
	var otherArgs []string
	skipNext := false

	for i, arg := range args {
		if skipNext {
			skipNext = false
			otherArgs = append(otherArgs, arg)
			continue
		}

		// Check if this arg is a flag that takes a value
		if strings.HasPrefix(arg, "-") {
			otherArgs = append(otherArgs, arg)
			// Flags that take next arg as value
			if arg == "-o" || arg == "-p" || arg == "-D" || arg == "-I" ||
				arg == "-importcfg" || arg == "-embedcfg" || arg == "-lang" ||
				arg == "-buildid" || arg == "-goversion" || arg == "-c" ||
				arg == "-trimpath" {
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					skipNext = true
				}
			}
			continue
		}

		// Check if it's a .go file
		if strings.HasSuffix(arg, ".go") {
			goFiles = append(goFiles, arg)
		} else {
			otherArgs = append(otherArgs, arg)
		}
	}

	// If no Go files found, just run the compile as-is
	if len(goFiles) == 0 {
		return runTool(compilePath, args)
	}

	// Check if this is a test file or internal package that we should skip
	if shouldSkipPackage(goFiles) {
		return runTool(compilePath, args)
	}

	// Run escape analysis on the source files
	escapeInfo, err := instrument.GetEscapeInfoForFiles(goFiles)
	if err != nil {
		// Escape analysis failed - continue without it
		// This can happen for incomplete packages or syntax errors
		escapeInfo = nil
	}

	// Create temp directory for instrumented files
	tempDir, err := os.MkdirTemp("", "racedetector-toolexec-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "toolexec: failed to create temp dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Instrument each source file
	instrumentedFiles := make([]string, 0, len(goFiles))
	for _, srcPath := range goFiles {
		// Skip _test.go files for now (complex handling)
		if strings.HasSuffix(srcPath, "_test.go") {
			instrumentedFiles = append(instrumentedFiles, srcPath)
			continue
		}

		// Instrument the file
		opts := instrument.InstrumentOptions{
			EscapeInfo: escapeInfo,
			Filename:   filepath.Base(srcPath),
		}
		result, err := instrument.InstrumentFileWithOptions(srcPath, nil, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "toolexec: failed to instrument %s: %v\n", srcPath, err)
			return 1
		}

		// Handle CGO files - use original
		if result.Stats.CGOSkipped {
			instrumentedFiles = append(instrumentedFiles, srcPath)
			continue
		}

		// Write instrumented file to temp directory
		outPath := filepath.Join(tempDir, filepath.Base(srcPath))
		if err := os.WriteFile(outPath, []byte(result.Code), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "toolexec: failed to write %s: %v\n", outPath, err)
			return 1
		}

		instrumentedFiles = append(instrumentedFiles, outPath)
	}

	// Build new argument list with instrumented files
	newArgs := append(otherArgs, instrumentedFiles...)

	// Run the real compile
	return runTool(compilePath, newArgs)
}

// shouldSkipPackage returns true if the package should not be instrumented.
//
// We skip:
//   - Standard library packages (all packages in GOROOT)
//   - Our own race detector packages
//   - Test packages (handled separately)
func shouldSkipPackage(goFiles []string) bool {
	if len(goFiles) == 0 {
		return true
	}

	// Check first file's path for patterns to skip
	firstFile := goFiles[0]

	// Skip Go standard library (GOROOT paths)
	// Standard library files are in paths like:
	//   C:\Users\...\go1.25.3\src\internal\...
	//   /usr/local/go/src/...
	// They contain "go" + version + "src" or just "go/src"
	if strings.Contains(firstFile, "\\go1.") && strings.Contains(firstFile, "\\src\\") {
		return true
	}
	if strings.Contains(firstFile, "/go1.") && strings.Contains(firstFile, "/src/") {
		return true
	}
	if strings.Contains(firstFile, "/go/src/") || strings.Contains(firstFile, "\\go\\src\\") {
		return true
	}

	// Skip our own race detector runtime packages (but NOT examples or tests)
	// Only skip internal/race, race/, cmd/racedetector packages
	if (strings.Contains(firstFile, "internal/race") ||
		strings.Contains(firstFile, "internal\\race") ||
		strings.Contains(firstFile, "/race/") ||
		strings.Contains(firstFile, "\\race\\") ||
		strings.Contains(firstFile, "cmd/racedetector") ||
		strings.Contains(firstFile, "cmd\\racedetector")) &&
		!strings.Contains(firstFile, "examples") {
		return true
	}

	return false
}

// runTool executes a tool and returns its exit code.
func runTool(toolPath string, args []string) int {
	cmd := exec.Command(toolPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}
