// Package runtime provides runtime library linking for instrumented code.
//
// This package handles injecting our Pure-Go race detector runtime into
// instrumented Go programs. It provides mechanisms to ensure the runtime
// is linked and initialized properly.
package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"golang.org/x/mod/modfile"
)

// Version is the current racedetector version for go.mod requires.
// This is set at build time via ldflags for releases.
// Default "dev" is used for local development (with replace directive).
//
// Set via: -ldflags "-X github.com/kolkov/racedetector/cmd/racedetector/runtime.Version=v0.7.0"
var Version = "dev"

// GetVersion returns the effective version string with "v" prefix for go.mod.
// Priority:
//  1. ldflags-injected Version (from GoReleaser builds)
//  2. Build info version (from "go install @version")
//  3. Default "dev" (for local development)
func GetVersion() string {
	version := Version
	if version == "dev" {
		// Fallback: try to get version from build info (go install @version)
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				version = info.Main.Version
			}
		}
	}
	// Ensure version has "v" prefix for go.mod compatibility
	if version != "dev" && !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

// GetRuntimePackagePath returns the import path for the race detector runtime.
//
// This is the package that instrumented code will import to access
// RaceRead, RaceWrite, and other race detection functions.
//
// Uses public API wrapper instead of internal package for standalone tool compatibility.
//
// Returns: "github.com/kolkov/racedetector/race"
func GetRuntimePackagePath() string {
	return "github.com/kolkov/racedetector/race"
}

// GetRuntimeInitCode returns Go code to initialize the race detector.
//
// This code should be injected at the beginning of the main() function
// to ensure the detector is properly initialized before any memory accesses.
//
// Returns:
//   - Go code string to initialize race detector
//
// Example output:
//
//	race.Init()
//	defer race.Fini()
func GetRuntimeInitCode() string {
	return `race.Init()
defer race.Fini()`
}

// ValidateRuntimeAvailable checks if the runtime library is available.
//
// This verifies that the race detector runtime package can be found
// and imported. If the package is missing, it provides instructions
// for installing it.
//
// Returns:
//   - nil if runtime is available
//   - error with installation instructions if missing
func ValidateRuntimeAvailable() error {
	// Check if we're in development (running from source)
	// In that case, the runtime is in internal/race/api
	projectRoot, err := findProjectRoot()
	if err == nil {
		runtimePath := filepath.Join(projectRoot, "internal", "race", "api")
		if _, err := os.Stat(runtimePath); err == nil {
			// Runtime found in development tree
			return nil
		}
	}

	// Check if runtime is installed in GOPATH/go modules
	// For now, we assume it's available since we're in the same repository
	// In production, this would check: go list github.com/.../api

	return nil
}

// findProjectRoot finds the root directory of the racedetector project.
//
// This walks up the directory tree from the current executable location
// looking for our specific project marker (internal/race/api directory).
// We don't just look for any go.mod because that would match the user's project.
//
// Returns:
//   - Project root path
//   - Error if root cannot be found
func findProjectRoot() (string, error) {
	// Start from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up looking for internal/race/api (our specific runtime marker)
	dir := cwd
	for {
		// Check for internal/race/api (our runtime - THIS IS THE KEY MARKER)
		runtimePath := filepath.Join(dir, "internal", "race", "api")
		if _, err := os.Stat(runtimePath); err == nil {
			return dir, nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root without finding project
			break
		}
		dir = parent
	}

	// Not found by walking up - try to find via executable path
	exePath, err := os.Executable()
	if err == nil {
		// Executable might be in project root or bin directory
		exeDir := filepath.Dir(exePath)
		candidates := []string{
			exeDir,                             // racedetector.exe in project root
			filepath.Dir(exeDir),               // racedetector.exe in bin/
			filepath.Dir(filepath.Dir(exeDir)), // deeper nesting
		}
		for _, candidate := range candidates {
			runtimePath := filepath.Join(candidate, "internal", "race", "api")
			if _, err := os.Stat(runtimePath); err == nil {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("could not find racedetector project root")
}

// findOriginalGoMod finds the go.mod file of the project being instrumented.
//
// This walks up from the given directory looking for go.mod file.
// This is different from findProjectRoot which finds racedetector's root.
//
// Parameters:
//   - startDir: Directory to start searching from (usually the source file's directory)
//
// Returns:
//   - Path to go.mod file
//   - Empty string if no go.mod found
func findOriginalGoMod(startDir string) string {
	dir := startDir
	for {
		modPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(modPath); err == nil {
			return modPath
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}
	return ""
}

// BuildFlags returns additional flags needed for building instrumented code.
//
// These flags ensure the runtime library is linked correctly and
// initialization code runs.
//
// Returns:
//   - Slice of build flags to pass to 'go build'
//
// Example:
//
//	flags := BuildFlags()
//	// flags = ["-tags=race", ...]
func BuildFlags() []string {
	// For now, no special flags needed
	// In future, might add:
	// - Custom build tags
	// - Linker flags
	// - Optimization flags
	return []string{}
}

// ModFileOverlay creates a temporary go.mod overlay for instrumented code.
//
// When instrumenting code outside the racedetector project, we need to
// ensure it can import our runtime. This creates a go.mod overlay that
// either replaces the remote import with a local path (development mode)
// or requires the published package (CI/published mode).
//
// It also preserves replace directives from the original project's go.mod,
// converting relative paths to absolute paths (since the temp directory
// has a different working directory).
//
// Parameters:
//   - tempDir: Temporary directory where instrumented code is being built
//   - sourceDir: Directory of the source file being instrumented (to find original go.mod)
//
// Returns:
//   - Path to overlay file (for -modfile flag)
//   - Error if overlay creation fails
func ModFileOverlay(tempDir, sourceDir string) (string, error) {
	// Build go.mod content
	var content strings.Builder
	content.WriteString("module instrumented\n\n")
	content.WriteString("go 1.24\n\n")

	// Check if we're in development mode (running from source)
	projectRoot, err := findProjectRoot()
	if err == nil {
		// Development mode - use local replace directive
		content.WriteString("require github.com/kolkov/racedetector v0.0.0\n\n")
		fmt.Fprintf(&content, "replace github.com/kolkov/racedetector => %s\n", projectRoot)
	} else {
		// Published mode (CI, installed via go install) - require published package
		fmt.Fprintf(&content, "require github.com/kolkov/racedetector %s\n", GetVersion())
	}

	// Find and parse original project's go.mod to:
	// 1. Add replace directive for the original module → ./src (so internal imports work)
	// 2. Copy existing replace directives from original go.mod
	if sourceDir != "" {
		originalGoMod := findOriginalGoMod(sourceDir)
		if originalGoMod != "" {
			// Get original module name and add replace directive
			originalModuleName := getModuleName(originalGoMod)
			// Don't add replace for our own module (racedetector) - it's already handled above
			if originalModuleName != "" && originalModuleName != "github.com/kolkov/racedetector" {
				content.WriteString("\n// Replace original module with instrumented sources:\n")
				fmt.Fprintf(&content, "replace %s => ./src\n", originalModuleName)
			}

			// Copy existing replace directives from original go.mod
			replaceDirectives := extractReplaceDirectives(originalGoMod)
			if replaceDirectives != "" {
				content.WriteString("\n// Replace directives from original go.mod:\n")
				content.WriteString(replaceDirectives)
			}
		}
	}

	// Create go.mod in temp directory
	overlayPath := filepath.Join(tempDir, "go.mod.overlay")
	if err := os.WriteFile(overlayPath, []byte(content.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to create go.mod overlay: %w", err)
	}

	return overlayPath, nil
}

// getModuleName reads a go.mod file and returns the module name.
//
// Parameters:
//   - goModPath: Path to the go.mod file to parse
//
// Returns:
//   - Module name (e.g., "github.com/gogpu/wgpu") or empty string on error
func getModuleName(goModPath string) string {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	modFile, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return ""
	}

	if modFile.Module == nil {
		return ""
	}

	return modFile.Module.Mod.Path
}

// extractReplaceDirectives reads a go.mod file and extracts replace directives,
// converting relative paths to absolute paths.
//
// Parameters:
//   - goModPath: Path to the go.mod file to parse
//
// Returns:
//   - String containing replace directives with absolute paths
func extractReplaceDirectives(goModPath string) string {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	modFile, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return ""
	}

	if len(modFile.Replace) == 0 {
		return ""
	}

	goModDir := filepath.Dir(goModPath)
	var result strings.Builder

	for _, rep := range modFile.Replace {
		newPath := rep.New.Path

		// Check if it's a local path (relative or already absolute)
		// Local paths don't have a version and are filesystem paths
		if rep.New.Version == "" && isLocalPath(newPath) {
			// Convert relative path to absolute
			if !filepath.IsAbs(newPath) {
				absPath, err := filepath.Abs(filepath.Join(goModDir, newPath))
				if err == nil {
					newPath = absPath
				}
			}
		}

		// Write the replace directive
		if rep.Old.Version != "" {
			// Replace specific version: replace foo v1.0.0 => bar
			if rep.New.Version != "" {
				fmt.Fprintf(&result, "replace %s %s => %s %s\n",
					rep.Old.Path, rep.Old.Version, newPath, rep.New.Version)
			} else {
				fmt.Fprintf(&result, "replace %s %s => %s\n",
					rep.Old.Path, rep.Old.Version, newPath)
			}
		} else {
			// Replace all versions: replace foo => bar
			if rep.New.Version != "" {
				fmt.Fprintf(&result, "replace %s => %s %s\n",
					rep.Old.Path, newPath, rep.New.Version)
			} else {
				fmt.Fprintf(&result, "replace %s => %s\n",
					rep.Old.Path, newPath)
			}
		}
	}

	return result.String()
}

// isLocalPath checks if a path is a local filesystem path (not a module path).
//
// Local paths start with ./, ../, /, or a drive letter on Windows.
func isLocalPath(path string) bool {
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		return true
	}
	if filepath.IsAbs(path) {
		return true
	}
	// Windows drive letter check (e.g., C:\)
	if len(path) >= 2 && path[1] == ':' {
		return true
	}
	// Check if it looks like a relative path (contains path separator but no dots)
	// This handles cases like "subdir/module"
	if strings.ContainsAny(path, `/\`) && !strings.Contains(path, ".") {
		return true
	}
	return false
}

// InjectInitCalls injects Init/Fini calls into the main function.
//
// This is a placeholder for future implementation. In a full implementation,
// this would use AST transformation to insert race.Init() at the start of
// main() and defer race.Fini().
//
// For the MVP, we rely on users calling Init/Fini manually or provide
// a wrapper main function.
//
// Parameters:
//   - sourceCode: Original Go source code
//
// Returns:
//   - Modified source code with Init/Fini calls
//   - Error if injection fails
func InjectInitCalls(sourceCode string) (string, error) {
	// TODO: Implement AST-based injection
	// For MVP, return unchanged
	return sourceCode, nil
}
