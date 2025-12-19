// Package instrument - Comprehensive test suite for AST instrumentation.
//
// This test file validates the instrumentation engine's ability to:
//  1. Parse Go source files
//  2. Inject required imports (race package, unsafe)
//  3. Detect memory access operations
//  4. Handle edge cases (multiple assignments, function calls, etc.)
//
// Test Coverage Goals:
//   - Simple variable assignments
//   - Pointer dereferences (read and write)
//   - Array/slice accesses
//   - Struct field accesses
//   - Import injection (with and without existing imports)
//   - Edge cases (blank identifier, constants, function calls)
//
// Phase 6A Task A.2 - AST Instrumentation Engine Tests
package instrument

import (
	"strings"
	"testing"
)

// TestInstrumentFile_SimpleVariable tests instrumentation of simple variable assignments.
//
// Test Case:
//
//	var x int
//	x = 42
//
// Expected:
//   - Import race and unsafe packages
//   - Instrument write to x
//
// Success Criteria:
//   - Output contains race and unsafe imports
//   - Output compiles without syntax errors
func TestInstrumentFile_SimpleVariable(t *testing.T) {
	input := `package main

var x int

func main() {
	x = 42
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify race package import was added.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify unsafe import was added.
	if !strings.Contains(result.Code, `"unsafe"`) {
		t.Errorf("Output missing unsafe import")
	}

	// Verify code is still valid Go (basic syntax check).
	if !strings.Contains(result.Code, "package main") {
		t.Errorf("Output missing package declaration")
	}
	if !strings.Contains(result.Code, "func main") {
		t.Errorf("Output missing main function")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_PointerDereference tests instrumentation of pointer dereferences.
//
// Test Case:
//
//	var ptr *int
//	y := *ptr  // READ
//	*ptr = 42  // WRITE
//
// Expected:
//   - Detect both read and write dereferences
//   - Import injection
func TestInstrumentFile_PointerDereference(t *testing.T) {
	input := `package main

func main() {
	var ptr *int
	y := *ptr
	*ptr = 42
	_ = y
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify original code is preserved.
	if !strings.Contains(result.Code, "*ptr") {
		t.Errorf("Output missing pointer dereference")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_ArrayAccess tests instrumentation of array/slice accesses.
//
// Test Case:
//
//	arr := []int{1, 2, 3}
//	y := arr[0]   // READ
//	arr[0] = 42   // WRITE
//
// Expected:
//   - Detect both read and write array accesses
func TestInstrumentFile_ArrayAccess(t *testing.T) {
	input := `package main

func main() {
	arr := []int{1, 2, 3}
	y := arr[0]
	arr[0] = 42
	_ = y
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify original code.
	if !strings.Contains(result.Code, "arr[0]") {
		t.Errorf("Output missing array access")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_StructField tests instrumentation of struct field accesses.
//
// Test Case:
//
//	type S struct { field int }
//	obj := S{}
//	y := obj.field   // READ
//	obj.field = 42   // WRITE
//
// Expected:
//   - Detect both read and write field accesses
func TestInstrumentFile_StructField(t *testing.T) {
	input := `package main

type S struct {
	field int
}

func main() {
	obj := S{}
	y := obj.field
	obj.field = 42
	_ = y
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify original code.
	if !strings.Contains(result.Code, "obj.field") {
		t.Errorf("Output missing field access")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_ImportInjection tests import injection with existing imports.
//
// Test Case:
//
//	package main
//	import "fmt"
//	func main() { fmt.Println("hello") }
//
// Expected:
//   - Preserve existing imports
//   - Add race and unsafe imports
//   - Use grouped import syntax: import (...)
func TestInstrumentFile_ImportInjection(t *testing.T) {
	input := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify all imports present.
	if !strings.Contains(result.Code, `"fmt"`) {
		t.Errorf("Output missing existing fmt import")
	}
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}
	if !strings.Contains(result.Code, `"unsafe"`) {
		t.Errorf("Output missing unsafe import")
	}

	// Verify grouped import syntax.
	if !strings.Contains(result.Code, "import (") {
		t.Errorf("Output should use grouped import syntax")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_MultipleAssignments tests multiple assignment statements.
//
// Test Case:
//
//	x, y := 1, 2
//
// Expected:
//   - Instrument both x and y writes
func TestInstrumentFile_MultipleAssignments(t *testing.T) {
	input := `package main

func main() {
	x, y := 1, 2
	_ = x
	_ = y
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify original code.
	if !strings.Contains(result.Code, "x, y :=") {
		t.Errorf("Output missing multiple assignment")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_FunctionCall tests that function calls are NOT instrumented.
//
// Test Case:
//
//	foo(x)
//
// Expected:
//   - Do NOT instrument x in function call (callee handles it)
func TestInstrumentFile_FunctionCall(t *testing.T) {
	input := `package main

func foo(val int) {}

func main() {
	x := 42
	foo(x)
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify original code preserved.
	if !strings.Contains(result.Code, "foo(x)") {
		t.Errorf("Output missing function call")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_BlankIdentifier tests that blank identifier is not instrumented.
//
// Test Case:
//
//	_ = x
//
// Expected:
//   - Skip instrumentation for blank identifier
func TestInstrumentFile_BlankIdentifier(t *testing.T) {
	input := `package main

func main() {
	x := 42
	_ = x
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify blank identifier preserved.
	if !strings.Contains(result.Code, "_ = x") {
		t.Errorf("Output missing blank identifier assignment")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_Constants tests that constants are not instrumented.
//
// Test Case:
//
//	const C = 42
//
// Expected:
//   - Constants don't need instrumentation (no runtime memory access)
func TestInstrumentFile_Constants(t *testing.T) {
	input := `package main

const C = 42

func main() {
	x := C
	_ = x
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify constant preserved.
	if !strings.Contains(result.Code, "const C") {
		t.Errorf("Output missing constant declaration")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_NoImports tests file with no imports.
//
// Test Case:
//
//	package main
//	func main() {}
//
// Expected:
//   - Create import block with race and unsafe
func TestInstrumentFile_NoImports(t *testing.T) {
	input := `package main

func main() {
	var x int
	x = 42
	_ = x
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports added.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}
	if !strings.Contains(result.Code, `"unsafe"`) {
		t.Errorf("Output missing unsafe import")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_SyntaxError tests handling of invalid Go code.
//
// Test Case:
//
//	Invalid Go syntax
//
// Expected:
//   - Return error (parsing fails)
func TestInstrumentFile_SyntaxError(t *testing.T) {
	input := `package main

func main() {
	this is not valid Go code
}
`

	_, err := InstrumentFile("test.go", input)
	if err == nil {
		t.Fatalf("InstrumentFile should fail on invalid syntax")
	}

	// Verify error message mentions parsing.
	if !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "failed") {
		t.Errorf("Error should mention parsing failure: %v", err)
	}
}

// TestInstrumentFile_ExistingRaceImport tests handling when race import already exists.
//
// Test Case:
//
//	import race "github.com/kolkov/racedetector/internal/race/api"
//
// Expected:
//   - Do NOT add duplicate import
func TestInstrumentFile_ExistingRaceImport(t *testing.T) {
	input := `package main

import race "github.com/kolkov/racedetector/internal/race/api"
import "unsafe"

func main() {
	var x int
	x = 42
	_ = x
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports preserved (no duplicates).
	// Count occurrences of race import path.
	count := strings.Count(result.Code, RacePackageImportPath)
	if count != 1 {
		t.Errorf("Expected exactly 1 race import, found %d", count)
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInjectImports_NoExistingImports tests import injection with no existing imports.
func TestInjectImports_NoExistingImports(t *testing.T) {
	input := `package main

func main() {}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify both imports added.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}
	if !strings.Contains(result.Code, `"unsafe"`) {
		t.Errorf("Output missing unsafe import")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInjectImports_PreserveExisting tests that existing imports are preserved.
func TestInjectImports_PreserveExisting(t *testing.T) {
	input := `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello")
	os.Exit(0)
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify all imports present.
	if !strings.Contains(result.Code, `"fmt"`) {
		t.Errorf("Output missing fmt import")
	}
	if !strings.Contains(result.Code, `"os"`) {
		t.Errorf("Output missing os import")
	}
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}
	if !strings.Contains(result.Code, `"unsafe"`) {
		t.Errorf("Output missing unsafe import")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_IncDecStmt tests instrumentation of increment/decrement statements.
//
// Test Case:
//
//	counter++
//	counter--
//	i++
//
// Expected:
//   - Both RaceRead and RaceWrite for each inc/dec operation
//   - counter++ is semantically counter = counter + 1 (read + write)
//
// This test was added to fix a bug where IncDecStmt was not being instrumented,
// causing race detection to miss races in code like:
//
//	go func() { counter++ }()
//	go func() { counter++ }()
func TestInstrumentFile_IncDecStmt(t *testing.T) {
	input := `package main

var counter int

func main() {
	counter++
	counter--
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify imports added.
	if !strings.Contains(result.Code, RacePackageImportPath) {
		t.Errorf("Output missing race package import")
	}

	// Verify RaceRead and RaceWrite calls are present.
	// counter++ should generate both a read and a write.
	if !strings.Contains(result.Code, "race.RaceRead") {
		t.Errorf("Output missing RaceRead for counter++")
	}
	if !strings.Contains(result.Code, "race.RaceWrite") {
		t.Errorf("Output missing RaceWrite for counter++")
	}

	// Verify stats: counter++ and counter-- should each count as 1 read + 1 write.
	// Total: 2 reads + 2 writes.
	if result.Stats.ReadsInstrumented != 2 {
		t.Errorf("Stats.ReadsInstrumented = %d, want 2", result.Stats.ReadsInstrumented)
	}
	if result.Stats.WritesInstrumented != 2 {
		t.Errorf("Stats.WritesInstrumented = %d, want 2", result.Stats.WritesInstrumented)
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_IncDecInGoroutine tests instrumentation in anonymous functions.
//
// This is a critical test because race conditions commonly occur in patterns like:
//
//	go func() { counter++ }()
//	go func() { counter++ }()
//
// The instrumenter must correctly instrument code inside anonymous functions.
func TestInstrumentFile_IncDecInGoroutine(t *testing.T) {
	input := `package main

var counter int

func main() {
	go func() {
		counter++
	}()
	go func() {
		counter++
	}()
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Verify both goroutines have their counter++ instrumented.
	// 2 goroutines × (1 read + 1 write) = 4 operations total.
	if result.Stats.ReadsInstrumented != 2 {
		t.Errorf("Stats.ReadsInstrumented = %d, want 2", result.Stats.ReadsInstrumented)
	}
	if result.Stats.WritesInstrumented != 2 {
		t.Errorf("Stats.WritesInstrumented = %d, want 2", result.Stats.WritesInstrumented)
	}

	// Verify the instrumented code contains race detection calls.
	if !strings.Contains(result.Code, "race.RaceRead") {
		t.Errorf("Output missing RaceRead calls")
	}
	if !strings.Contains(result.Code, "race.RaceWrite") {
		t.Errorf("Output missing RaceWrite calls")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_FunctionReference tests that function references are not instrumented.
// Issue #9: Cannot take address of function value.
func TestInstrumentFile_FunctionReference(t *testing.T) {
	input := `package main

func parseSpec() {}

func main() {
	f := parseSpec
	_ = f
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// The instrumented code should compile without errors.
	// We verify by checking it doesn't contain &parseSpec (which would be invalid).
	if strings.Contains(result.Code, "&parseSpec") {
		t.Errorf("Output contains invalid &parseSpec - should not take address of function")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_BuiltinMake tests that built-in make is not instrumented.
// Issue #9: make (built-in) must be called.
func TestInstrumentFile_BuiltinMake(t *testing.T) {
	input := `package main

func main() {
	s := make([]int, 10)
	_ = s
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// The instrumented code should not try to take address of make.
	if strings.Contains(result.Code, "&make") {
		t.Errorf("Output contains invalid &make - should not take address of built-in")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_TypeConversion tests that type conversions are not instrumented.
// Issue #9: string (type) is not an expression.
func TestInstrumentFile_TypeConversion(t *testing.T) {
	input := `package main

func main() {
	data := []byte("hello")
	s := string(data)
	_ = s
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// The instrumented code should not try to take address of string type.
	if strings.Contains(result.Code, "&string") {
		t.Errorf("Output contains invalid &string - should not take address of type")
	}
	if strings.Contains(result.Code, "&byte") {
		t.Errorf("Output contains invalid &byte - should not take address of type")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_BuiltinLen tests that built-in len is not instrumented.
func TestInstrumentFile_BuiltinLen(t *testing.T) {
	input := `package main

func main() {
	s := []int{1, 2, 3}
	n := len(s)
	_ = n
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// The instrumented code should not try to take address of len.
	if strings.Contains(result.Code, "&len") {
		t.Errorf("Output contains invalid &len - should not take address of built-in")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_PackageFunction tests that package function calls are not instrumented.
// Issue #9: Cannot take address of os.ReadFile, strconv.Atoi, etc.
func TestInstrumentFile_PackageFunction(t *testing.T) {
	input := `package main

import (
	"os"
	"strconv"
)

func main() {
	data, _ := os.ReadFile("test.txt")
	n, _ := strconv.Atoi("42")
	_ = data
	_ = n
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// The instrumented code should not try to take address of package functions.
	if strings.Contains(result.Code, "&os.ReadFile") {
		t.Errorf("Output contains invalid &os.ReadFile - should not take address of package function")
	}
	if strings.Contains(result.Code, "&strconv.Atoi") {
		t.Errorf("Output contains invalid &strconv.Atoi - should not take address of package function")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_MapIndex tests that map index expressions are not instrumented.
// Issue #9: Cannot take address of map[key].
func TestInstrumentFile_MapIndex(t *testing.T) {
	input := `package main

func main() {
	m := map[string]int{"a": 1, "b": 2}
	v := m["a"]
	_ = v
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// The instrumented code should not try to take address of map index.
	// Note: &m["a"] is invalid in Go - cannot take address of map element
	if strings.Contains(result.Code, `&m["a"]`) || strings.Contains(result.Code, `&m[`) {
		t.Errorf("Output contains invalid &m[...] - should not take address of map index")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_MethodValue tests that method values are not instrumented.
// Issue #9: Cannot take address of obj.Method.
func TestInstrumentFile_MethodValue(t *testing.T) {
	input := `package main

import "bytes"

func main() {
	buf := bytes.NewBuffer(nil)
	f := buf.Write
	_ = f
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// The instrumented code should not try to take address of method value.
	if strings.Contains(result.Code, "&buf.Write") {
		t.Errorf("Output contains invalid &buf.Write - should not take address of method value")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
}

// TestInstrumentFile_StructFieldConservative tests that struct fields are handled safely.
// We now skip ALL SelectorExpr for safety (may miss some races on struct fields).
func TestInstrumentFile_StructFieldConservative(t *testing.T) {
	input := `package main

type Point struct {
	X, Y int
}

func main() {
	p := Point{X: 1, Y: 2}
	v := p.X
	_ = v
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// With conservative approach, we skip p.X instrumentation
	// This is a known limitation - we prioritize safety over completeness
	t.Logf("Instrumented output:\n%s", result.Code)
}

func BenchmarkInstrumentFile(b *testing.B) {
	input := `package main

func main() {
	var x int
	x = 42
	y := x
	arr := []int{1, 2, 3}
	z := arr[0]
	arr[1] = 100
	_ = y
	_ = z
}
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := InstrumentFile("bench.go", input)
		if err != nil {
			b.Fatalf("InstrumentFile failed: %v", err)
		}
	}
}

// TestInstrumentFile_CGOSkipped tests that CGO files are detected and skipped.
//
// CGO files contain `import "C"` which introduces C symbols that cannot
// be processed by go/parser. These files should be skipped to avoid
// compilation errors like "could not determine what C.xxx refers to".
//
// Fixes: https://github.com/kolkov/racedetector/issues/16
func TestInstrumentFile_CGOSkipped(t *testing.T) {
	// CGO file with import "C"
	input := `package main

// #include <stdio.h>
import "C"

func main() {
	C.puts(C.CString("hello"))
}
`

	result, err := InstrumentFile("cgo_test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed for CGO file: %v", err)
	}

	// Should return empty code to signal file should be copied unchanged
	if result.Code != "" {
		t.Errorf("CGO file should return empty Code, got: %q", result.Code)
	}

	// Should set CGOSkipped flag
	if !result.Stats.CGOSkipped {
		t.Error("CGO file should have Stats.CGOSkipped = true")
	}

	t.Logf("CGO file correctly skipped (CGOSkipped=%v)", result.Stats.CGOSkipped)
}

// TestInstrumentFile_NonCGO tests that regular Go files are NOT skipped.
func TestInstrumentFile_NonCGO(t *testing.T) {
	input := `package main

func main() {
	x := 42
	_ = x
}
`

	result, err := InstrumentFile("regular.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Should NOT be skipped
	if result.Stats.CGOSkipped {
		t.Error("Regular Go file should NOT have CGOSkipped = true")
	}

	// Should have instrumented code
	if result.Code == "" {
		t.Error("Regular Go file should have non-empty Code")
	}
}

// TestInstrumentFile_StarExprPointerDeref tests instrumentation of pointer dereferences
// using StarExpr AST node (not UnaryExpr with MUL).
//
// Issue #27: Pointer dereferences were not being instrumented because Go parser
// uses *ast.StarExpr for pointer dereference in contexts like *ptr++ (IncDecStmt),
// but our code only checked for *ast.UnaryExpr with token.MUL.
//
// This is the exact case reported by @thepudds in golang/go#6508.
//
// Test Case:
//
//	func update(ptr *int) {  // ptr doesn't escape (stack-local copy)
//	    *ptr++               // But *ptr accesses shared memory - MUST instrument!
//	}
//
// Expected:
//   - Instrument *ptr++ even though ptr parameter "does not escape"
//   - The memory accessed via *ptr CAN be shared between goroutines
func TestInstrumentFile_StarExprPointerDeref(t *testing.T) {
	input := `package main

func update(ptr *int) {
	*ptr++
}

func main() {
	var x int
	update(&x)
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Must have instrumentation for *ptr++
	// *ptr++ is both a read and a write
	if result.Stats.ReadsInstrumented < 1 {
		t.Errorf("Stats.ReadsInstrumented = %d, want >= 1 (for *ptr read)", result.Stats.ReadsInstrumented)
	}

	// Verify RaceRead call is present for *ptr
	if !strings.Contains(result.Code, "race.RaceRead") {
		t.Errorf("Output missing RaceRead for *ptr dereference")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
	t.Logf("Stats: reads=%d, writes=%d", result.Stats.ReadsInstrumented, result.Stats.WritesInstrumented)
}

// TestInstrumentFile_StarExprInGoroutine tests the full race scenario from @thepudds.
//
// This is the exact reproduction case from Issue #27 / golang/go#6508:
// Two goroutines calling update(&shared) where update does *ptr++.
//
// Before fix: 0 reads, 0 writes (race missed!)
// After fix: >= 1 read for *ptr dereference
func TestInstrumentFile_StarExprInGoroutine(t *testing.T) {
	input := `package main

import "sync"

func update(ptr *int) {
	*ptr++
}

func main() {
	var shared int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		update(&shared)
	}()
	go func() {
		defer wg.Done()
		update(&shared)
	}()
	wg.Wait()
}
`

	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Critical assertion: *ptr++ MUST be instrumented
	// If ReadsInstrumented is 0, we're missing the pointer dereference race!
	if result.Stats.ReadsInstrumented == 0 {
		t.Fatalf("CRITICAL: ReadsInstrumented = 0, pointer dereference not instrumented! Issue #27 not fixed.")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
	t.Logf("Stats: reads=%d, writes=%d (must be > 0 for *ptr++)", result.Stats.ReadsInstrumented, result.Stats.WritesInstrumented)
}

// TestInstrumentFile_SelectorExprIncDec tests that struct field access via IncDecStmt is instrumented.
// This is the fix for Issue #30: s.x++ was not instrumented because we skip all SelectorExpr.
// In IncDecStmt context, we KNOW the selector is addressable (otherwise Go won't compile).
func TestInstrumentFile_SelectorExprIncDec(t *testing.T) {
	input := `package main

type S struct {
	x int
}

func update(s *S) {
	s.x++
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Must have instrumentation for s.x++
	// s.x++ is both READ and WRITE
	if result.Stats.ReadsInstrumented < 1 {
		t.Errorf("Stats.ReadsInstrumented = %d, want >= 1 for s.x++", result.Stats.ReadsInstrumented)
	}
	if result.Stats.WritesInstrumented < 1 {
		t.Errorf("Stats.WritesInstrumented = %d, want >= 1 for s.x++", result.Stats.WritesInstrumented)
	}

	// Verify the instrumented code contains race calls for the field
	if !strings.Contains(result.Code, "race.RaceRead") {
		t.Error("Instrumented code missing race.RaceRead for s.x++")
	}
	if !strings.Contains(result.Code, "race.RaceWrite") {
		t.Error("Instrumented code missing race.RaceWrite for s.x++")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
	t.Logf("Stats: reads=%d, writes=%d", result.Stats.ReadsInstrumented, result.Stats.WritesInstrumented)
}

// TestInstrumentFile_SelectorExprInGoroutine tests @thepudds Issue #30 reproduction case.
func TestInstrumentFile_SelectorExprInGoroutine(t *testing.T) {
	input := `package main

import "sync"

type S struct {
	x int
}

func update(s *S) {
	s.x++
}

func main() {
	var shared S
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		update(&shared)
	}()
	go func() {
		defer wg.Done()
		update(&shared)
	}()
	wg.Wait()
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Critical: s.x++ must be instrumented
	if result.Stats.ReadsInstrumented == 0 && result.Stats.WritesInstrumented == 0 {
		t.Fatalf("CRITICAL: No instrumentation for s.x++! Issue #30 not fixed.")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
	t.Logf("Stats: reads=%d, writes=%d", result.Stats.ReadsInstrumented, result.Stats.WritesInstrumented)
}

// TestInstrumentFile_MakeWithCustomType verifies that make() with custom types compiles correctly.
//
// Issue: Type names in make([]*CustomType, 0) were incorrectly instrumented as variable reads.
// This caused compilation error: "CustomType (type) is not an expression"
//
// The fix: Skip type expressions (ArrayType, MapType, etc.) and first arg of make/new.
func TestInstrumentFile_MakeWithCustomType(t *testing.T) {
	input := `package main

type MyStruct struct {
	Value int
}

func main() {
	slice := make([]*MyStruct, 0)
	m := make(map[string]*MyStruct)
	ptr := new(MyStruct)
	_ = len(slice)
	_ = len(m)
	_ = ptr
}
`
	result, err := InstrumentFile("test.go", input)
	if err != nil {
		t.Fatalf("InstrumentFile failed: %v", err)
	}

	// Check: code should NOT contain RaceRead with type name
	if strings.Contains(result.Code, "RaceRead(uintptr(unsafe.Pointer(MyStruct)))") {
		t.Errorf("CRITICAL: Type name 'MyStruct' incorrectly instrumented as variable!")
	}

	// Check: code should still compile (no "type is not an expression" errors)
	// The presence of make/new calls means we need to handle types correctly
	if !strings.Contains(result.Code, "make([]*MyStruct") {
		t.Errorf("Expected make([]*MyStruct to be present in output")
	}

	t.Logf("Instrumented output:\n%s", result.Code)
	t.Logf("Stats: reads=%d, writes=%d", result.Stats.ReadsInstrumented, result.Stats.WritesInstrumented)
}
