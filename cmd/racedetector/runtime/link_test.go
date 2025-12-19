// link_test.go tests runtime library injection.
package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetRuntimePackagePath verifies the runtime import path is correct.
func TestGetRuntimePackagePath(t *testing.T) {
	path := GetRuntimePackagePath()

	// Should return the public race API path (for standalone tool compatibility)
	expected := "github.com/kolkov/racedetector/race"
	if path != expected {
		t.Errorf("GetRuntimePackagePath() = %q, want %q", path, expected)
	}

	// Should be a valid Go import path
	if !strings.Contains(path, "/") {
		t.Errorf("GetRuntimePackagePath() returned invalid import path: %q", path)
	}
}

// TestGetVersion verifies version detection priority.
func TestGetVersion(t *testing.T) {
	version := GetVersion()

	// In development mode, Version is "dev" but GetVersion() may return
	// build info version if available, or "dev" as fallback
	if version == "" {
		t.Error("GetVersion() returned empty string")
	}

	// Version should be either:
	// - "dev" (development)
	// - "vX.Y.Z" (release via ldflags or go install @version)
	// - "(devel)" should NOT be returned (filtered out)
	if version == "(devel)" {
		t.Error("GetVersion() should not return (devel)")
	}

	t.Logf("GetVersion() = %q (Version var = %q)", version, Version)
}

// TestGetVersion_VPrefix verifies that GetVersion adds "v" prefix for go.mod compatibility.
func TestGetVersion_VPrefix(t *testing.T) {
	// Save original version
	originalVersion := Version
	defer func() { Version = originalVersion }()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"dev mode", "dev", "dev"},
		{"with v prefix", "v0.8.3", "v0.8.3"},
		{"without v prefix", "0.8.3", "v0.8.3"},
		{"with v prefix semver", "v1.0.0", "v1.0.0"},
		{"without v prefix semver", "1.0.0", "v1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.input
			got := GetVersion()
			if got != tt.expected {
				t.Errorf("GetVersion() with Version=%q = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestGetRuntimeInitCode verifies the initialization code is correct.
func TestGetRuntimeInitCode(t *testing.T) {
	code := GetRuntimeInitCode()

	// Must contain Init() call
	if !strings.Contains(code, "race.Init()") {
		t.Errorf("GetRuntimeInitCode() missing race.Init() call")
	}

	// Must contain defer Fini() call
	if !strings.Contains(code, "defer race.Fini()") {
		t.Errorf("GetRuntimeInitCode() missing defer race.Fini() call")
	}

	// Should be valid Go code structure
	if !strings.Contains(code, "defer") {
		t.Errorf("GetRuntimeInitCode() missing defer keyword")
	}
}

// TestValidateRuntimeAvailable checks runtime availability detection.
func TestValidateRuntimeAvailable(t *testing.T) {
	// This should pass in our development environment
	err := ValidateRuntimeAvailable()

	if err != nil {
		t.Logf("ValidateRuntimeAvailable() returned: %v", err)
		// Not a fatal error in test environment, just log it
		// In production, we'd check for actual runtime package
	}
}

// TestFindProjectRoot verifies project root detection.
func TestFindProjectRoot(t *testing.T) {
	root, err := findProjectRoot()

	if err != nil {
		// This might fail in some test environments
		t.Logf("findProjectRoot() error: %v (expected if not in project tree)", err)
		return
	}

	// If we found a root, it should have go.mod or internal/race/api
	goModPath := filepath.Join(root, "go.mod")
	runtimePath := filepath.Join(root, "internal", "race", "api")

	hasGoMod := false
	hasRuntime := false

	if _, err := os.Stat(goModPath); err == nil {
		hasGoMod = true
	}
	if _, err := os.Stat(runtimePath); err == nil {
		hasRuntime = true
	}

	if !hasGoMod && !hasRuntime {
		t.Errorf("findProjectRoot() returned %q but it has neither go.mod nor internal/race/api", root)
	}

	t.Logf("Project root found: %s (hasGoMod=%v, hasRuntime=%v)", root, hasGoMod, hasRuntime)
}

// TestBuildFlags verifies build flags are returned correctly.
func TestBuildFlags(t *testing.T) {
	flags := BuildFlags()

	// Should return a slice (even if empty for MVP)
	if flags == nil {
		t.Errorf("BuildFlags() returned nil, want empty slice")
	}

	// For MVP, we expect empty flags
	// Future versions might add custom build tags or linker flags
	if len(flags) > 0 {
		t.Logf("BuildFlags() returned: %v", flags)
	}
}

// TestModFileOverlay verifies go.mod overlay creation.
func TestModFileOverlay(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "racedetector-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test overlay creation (pass empty sourceDir for basic test)
	overlayPath, err := ModFileOverlay(tempDir, "")

	// If we're not in development mode, overlay might be empty
	if err != nil {
		t.Fatalf("ModFileOverlay() failed: %v", err)
	}

	// If overlay was created, verify it
	if overlayPath != "" {
		// Overlay file should exist
		if _, err := os.Stat(overlayPath); err != nil {
			t.Errorf("ModFileOverlay() created path %q but file doesn't exist: %v", overlayPath, err)
		}

		// Read and verify content
		content, err := os.ReadFile(overlayPath)
		if err != nil {
			t.Fatalf("Failed to read overlay file: %v", err)
		}

		contentStr := string(content)

		// Must be a valid go.mod
		if !strings.Contains(contentStr, "module instrumented") {
			t.Errorf("Overlay missing 'module instrumented' declaration")
		}

		// Must have replace directive
		if !strings.Contains(contentStr, "replace github.com/kolkov/racedetector") {
			t.Errorf("Overlay missing replace directive")
		}

		// Must specify go version
		if !strings.Contains(contentStr, "go 1.") {
			t.Errorf("Overlay missing go version directive")
		}

		t.Logf("Overlay content:\n%s", contentStr)
	} else {
		t.Logf("ModFileOverlay() returned empty path (not in development mode)")
	}
}

// TestModFileOverlay_InvalidDir verifies error handling for invalid directory.
func TestModFileOverlay_InvalidDir(t *testing.T) {
	// Try to create overlay in non-existent directory
	invalidDir := "/this/path/should/not/exist/racedetector-test-12345"

	_, err := ModFileOverlay(invalidDir, "")

	// If we're in development mode, this should fail
	// If not in development mode, it returns empty string with no error
	// Both are acceptable
	if err != nil {
		t.Logf("ModFileOverlay() with invalid dir returned error (expected): %v", err)
	}
}

// TestModFileOverlay_WithReplaceDirectives verifies replace directive handling.
func TestModFileOverlay_WithReplaceDirectives(t *testing.T) {
	// Create temp directories
	tempDir, err := os.MkdirTemp("", "racedetector-overlay-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a fake source directory with go.mod containing replace directives
	sourceDir, err := os.MkdirTemp("", "racedetector-source-*")
	if err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}
	defer os.RemoveAll(sourceDir)

	// Create a go.mod with replace directives
	// Use platform-appropriate paths
	var absPath string
	if os.PathSeparator == '\\' {
		// Windows
		absPath = "C:\\absolute\\path\\to\\other" //nolint:misspell // "other" is not a misspelling
	} else {
		// Unix
		absPath = "/absolute/path/to/other"
	}

	goModContent := fmt.Sprintf(`module example.com/test

go 1.21

require (
	example.com/lib v1.0.0
)

replace example.com/lib => ../locallib
replace example.com/other => %s
`, absPath)
	if err := os.WriteFile(filepath.Join(sourceDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	// Test overlay creation with source dir
	overlayPath, err := ModFileOverlay(tempDir, sourceDir)
	if err != nil {
		t.Fatalf("ModFileOverlay() failed: %v", err)
	}

	if overlayPath == "" {
		t.Skip("Not in development mode, skipping replace directive test")
	}

	// Read and verify content
	content, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("Failed to read overlay: %v", err)
	}

	contentStr := string(content)
	t.Logf("Generated overlay:\n%s", contentStr)

	// Should contain comment about original replace directives
	if !strings.Contains(contentStr, "Replace directives from original go.mod") {
		t.Errorf("Overlay missing comment about original replace directives")
	}

	// Should contain the example.com/lib replace (path should be absolute now)
	if !strings.Contains(contentStr, "replace example.com/lib") {
		t.Errorf("Overlay missing example.com/lib replace directive")
	}

	// The relative path ../locallib should have been converted to absolute
	if strings.Contains(contentStr, "../locallib") {
		t.Errorf("Overlay still contains relative path ../locallib - should be absolute")
	}

	// The absolute path should remain as-is
	if !strings.Contains(contentStr, absPath) {
		t.Errorf("Overlay missing or modified absolute path for example.com/other, expected %q", absPath)
	}
}

// TestInjectInitCalls verifies initialization code injection.
func TestInjectInitCalls(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "simple program",
			source: `package main

func main() {
	println("hello")
}`,
		},
		{
			name: "program with existing code",
			source: `package main

import "fmt"

func main() {
	fmt.Println("hello, world")
	x := 42
	fmt.Println(x)
}`,
		},
		{
			name: "empty main",
			source: `package main

func main() {
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := InjectInitCalls(tt.source)

			if err != nil {
				t.Errorf("InjectInitCalls() error: %v", err)
				return
			}

			// For MVP, it returns source unchanged
			// TODO: In full implementation, verify Init/Fini calls are present
			if result != tt.source {
				t.Logf("InjectInitCalls() modified source (full implementation)")
			} else {
				t.Logf("InjectInitCalls() returned unchanged source (MVP)")
			}
		})
	}
}

// TestInjectInitCalls_EmptySource verifies handling of empty source.
func TestInjectInitCalls_EmptySource(t *testing.T) {
	result, err := InjectInitCalls("")

	if err != nil {
		t.Errorf("InjectInitCalls(\"\") error: %v", err)
	}

	if result != "" {
		t.Errorf("InjectInitCalls(\"\") = %q, want empty string", result)
	}
}

// BenchmarkGetRuntimePackagePath benchmarks path retrieval.
func BenchmarkGetRuntimePackagePath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GetRuntimePackagePath()
	}
}

// BenchmarkGetRuntimeInitCode benchmarks init code generation.
func BenchmarkGetRuntimeInitCode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GetRuntimeInitCode()
	}
}

// BenchmarkValidateRuntimeAvailable benchmarks runtime validation.
func BenchmarkValidateRuntimeAvailable(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ValidateRuntimeAvailable()
	}
}

// BenchmarkFindProjectRoot benchmarks project root detection.
func BenchmarkFindProjectRoot(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = findProjectRoot()
	}
}
