// Package instrument implements AST-level instrumentation for automatic
// race detection call insertion.
//
// This package provides the core functionality for the racedetector standalone
// tool. It parses Go source files, walks the AST to find memory access
// operations, and inserts race.RaceRead() and race.RaceWrite() calls automatically.
//
// Algorithm:
//  1. Parse Go source file using go/parser
//  2. Walk AST to find memory accesses (assignments, dereferences, etc.)
//  3. Insert race detection calls BEFORE each access
//  4. Inject required imports (race package, unsafe)
//  5. Generate instrumented code using go/printer
//
// Example Transformation:
//
//	// INPUT (original code):
//	var x int
//	x = 42
//	y := x
//
//	// OUTPUT (instrumented code):
//	import race "github.com/kolkov/racedetector/internal/race/api"
//	import "unsafe"
//	var x int
//	race.RaceWrite(uintptr(unsafe.Pointer(&x)))
//	x = 42
//	race.RaceRead(uintptr(unsafe.Pointer(&x)))
//	y := x
//
// Performance: Instrumentation happens at compile-time, not runtime, so
// performance is not critical. However, we aim for <1s per 1000 lines of code.
//
// Thread Safety: This package is NOT thread-safe. Callers must ensure
// single-threaded access or use external synchronization.
package instrument

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
)

const (
	// RacePackageImportPath is the import path for the race detector API.
	// This will be injected into instrumented files.
	// Uses public API wrapper instead of internal package for standalone tool compatibility.
	RacePackageImportPath = "github.com/kolkov/racedetector/race"

	// RacePackageAlias is the local package alias used in instrumented code.
	// race.RaceRead(), race.RaceWrite()
	RacePackageAlias = "race"
)

// InstrumentResult holds the result of instrumentation.
//
// This structure contains both the instrumented code and statistics
// about what was instrumented.
//
//nolint:revive // InstrumentResult is clear and descriptive despite stuttering
type InstrumentResult struct {
	Code  string          // Instrumented source code
	Stats InstrumentStats // Instrumentation statistics
}

// InstrumentFile instruments a single Go source file with race detection calls.
//
// This is the main entry point for AST-level instrumentation. It performs
// the following steps:
//
//  1. Parse the source file into an AST
//  2. Inject required imports (race package, unsafe)
//  3. Walk the AST to find memory access operations
//  4. Insert race detection calls before each access
//  5. Generate instrumented code as a string
//
// Parameters:
//   - filename: Path to the Go source file (used for error messages)
//   - src: Source code to instrument. Can be:
//   - nil: Read from filename
//   - []byte: Use provided bytes
//   - string: Use provided string
//   - io.Reader: Read from reader
//
// Returns:
//   - *InstrumentResult: Result containing code and statistics
//   - error: Parsing or instrumentation error, or nil on success
//
// Example:
//
//	result, err := InstrumentFile("main.go", nil)
//	if err != nil {
//	    log.Fatalf("Instrumentation failed: %v", err)
//	}
//	fmt.Printf("Instrumented %d reads, %d writes\n",
//	    result.Stats.ReadsInstrumented, result.Stats.WritesInstrumented)
//	// Write result.Code to temp file and compile
//
// Performance: Typical instrumentation time is 10-50ms per file (1000 lines).
//
// Error Handling:
//   - Returns error if file cannot be parsed (syntax errors)
//   - Returns error if AST walking fails
//   - Returns error if code generation fails
//
// Thread Safety: NOT thread-safe. Do not call concurrently on the same file.
//
//nolint:revive // InstrumentFile is the standard API naming for this operation
func InstrumentFile(filename string, src interface{}) (*InstrumentResult, error) {
	// Step 1: Parse source file into AST.
	// We use parser.ParseComments to preserve comments in the output.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file %s: %w", filename, err)
	}

	// Step 1.5: Check for CGO files and skip instrumentation.
	// CGO files use `import "C"` pseudo-package which cannot be processed
	// by go/parser for C symbols. Skip these files to avoid compilation errors.
	if isCGOFile(file) {
		return &InstrumentResult{
			Code:  "", // Empty code signals caller to copy file unchanged
			Stats: InstrumentStats{CGOSkipped: true},
		}, nil
	}

	// Step 2: Inject required imports at the top of the file.
	// This adds:
	//   - import race "github.com/kolkov/racedetector/internal/race/api"
	//   - import "unsafe"
	// If these imports already exist, injectImports handles conflicts gracefully.
	if err := injectImports(file); err != nil {
		return nil, fmt.Errorf("failed to inject imports: %w", err)
	}

	// Step 3: Walk the AST and instrument memory accesses.
	// This traverses the entire AST, finds memory access nodes, and
	// inserts race detection calls before each access.
	visitor, err := instrumentAST(fset, file)
	if err != nil {
		return nil, fmt.Errorf("failed to instrument AST: %w", err)
	}

	// Step 3.5: Get instrumentation statistics from visitor
	// This will be returned along with the instrumented code
	stats := visitor.GetStats()

	// Step 4: Generate Go source code from the modified AST.
	// We use go/printer to convert the AST back to source code.
	// The printer handles formatting and indentation automatically.
	var buf bytes.Buffer
	cfg := &printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 8,
	}
	if err := cfg.Fprint(&buf, fset, file); err != nil {
		return nil, fmt.Errorf("failed to generate code: %w", err)
	}

	// Step 5: Add init function to call race.Init() (MVP workaround)
	// TODO: In full implementation, inject Init/Fini into main() function via AST
	code := buf.String()
	code += `

// init initializes race detector (added by racedetector tool)
func init() {
	race.Init()
	_ = unsafe.Sizeof(0) // Ensure unsafe import is used
}
`

	return &InstrumentResult{
		Code:  code,
		Stats: stats,
	}, nil
}

// instrumentAST walks the AST and instruments memory access operations.
//
// This function serves as the coordination point for AST instrumentation.
// It delegates to visitor.go for the actual instrumentation logic.
//
// Algorithm:
//  1. Create an instrumentVisitor instance
//  2. Walk the AST using ast.Walk()
//  3. The visitor detects memory accesses and records instrumentation points
//  4. Apply instrumentation (insert race detection calls)
//
// Parameters:
//   - fset: File set for source position information
//   - file: AST to instrument (modified in place)
//
// Returns:
//   - *instrumentVisitor: Visitor containing stats and instrumentation points
//   - error: Instrumentation error, or nil on success
//
// Implementation Strategy (Task A.2):
// For MVP, we'll implement a two-pass approach:
//
//	Pass 1: Identify all memory access nodes and record their locations
//	Pass 2: Insert race detection calls at recorded locations
//
// This avoids modifying the AST while walking it, which can cause
// iteration issues.
//
// Thread Safety: NOT thread-safe (modifies AST in place).
func instrumentAST(fset *token.FileSet, file *ast.File) (*instrumentVisitor, error) {
	// Pass 1: Create visitor instance and walk the AST.
	// This identifies all memory access operations and records instrumentation points.
	visitor := newInstrumentVisitor(fset, file)
	ast.Walk(visitor, file)

	// Pass 2: Apply instrumentation - insert race detection calls into AST.
	// This modifies the AST in place by inserting race.RaceRead/RaceWrite calls
	// BEFORE each memory access operation identified in Pass 1.
	if err := visitor.ApplyInstrumentation(); err != nil {
		return nil, fmt.Errorf("failed to apply instrumentation: %w", err)
	}

	return visitor, nil
}

// isCGOFile checks if the given AST file contains `import "C"`.
//
// CGO files cannot be fully processed by go/parser because C symbols
// (like C.xxx) cannot be resolved. We skip instrumentation for these files
// to avoid compilation errors.
//
// Parameters:
//   - file: Parsed AST file to check
//
// Returns:
//   - true if file contains `import "C"`, false otherwise
func isCGOFile(file *ast.File) bool {
	for _, imp := range file.Imports {
		// Import path is stored as a quoted string, e.g., `"C"`
		if imp.Path != nil && imp.Path.Value == `"C"` {
			return true
		}
	}
	return false
}
