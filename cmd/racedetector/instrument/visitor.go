// Package instrument - AST visitor for memory access detection.
//
// This file implements the core instrumentation logic using the visitor pattern
// to walk the AST and insert race detection calls.
package instrument

import (
	"go/ast"
	"go/token"
)

// InstrumentStats tracks instrumentation statistics.
//
// This structure collects metrics during the instrumentation process,
// providing visibility into what was instrumented and what was skipped.
//
// Use Case:
// Enable with -v flag to see detailed instrumentation statistics:
//
//	racedetector build -v main.go
//	Instrumented main.go:
//	  - 15 writes instrumented
//	  - 23 reads instrumented
//	  - 5 items skipped (3 constants, 1 built-in, 1 literal)
//	  - 8 stack-local skipped (escape analysis)
//	  Total: 38 race detection calls inserted
//
// Performance Impact:
// Negligible - just integer increments during AST traversal.
//
// Thread Safety: NOT thread-safe (single-threaded instrumentation).
//
//nolint:revive // InstrumentStats is clear and descriptive despite stuttering
type InstrumentStats struct {
	ReadsInstrumented  int  // Number of read operations instrumented
	WritesInstrumented int  // Number of write operations instrumented
	ConstantsSkipped   int  // Number of constants skipped (const declarations)
	BuiltinsSkipped    int  // Number of built-in identifiers skipped (nil, true, false, iota)
	LiteralsSkipped    int  // Number of literals skipped (42, "hello", 3.14)
	BlanksSkipped      int  // Number of blank identifiers (_) skipped
	EscapeSkipped      int  // v0.8.0: Number of stack-local variables skipped (escape analysis)
	CGOSkipped         bool // True if file was skipped due to CGO (import "C")
}

// Total returns total number of instrumented accesses.
func (s *InstrumentStats) Total() int {
	return s.ReadsInstrumented + s.WritesInstrumented
}

// TotalSkipped returns total number of skipped items.
func (s *InstrumentStats) TotalSkipped() int {
	return s.ConstantsSkipped + s.BuiltinsSkipped + s.LiteralsSkipped + s.BlanksSkipped + s.EscapeSkipped
}

// instrumentVisitor implements ast.Visitor for instrumenting memory accesses.
//
// The visitor pattern allows us to traverse the AST and react to specific
// node types (assignments, dereferences, array accesses, struct fields).
//
// Strategy:
// We implement a simple approach that identifies memory access nodes but
// does NOT modify the AST directly during traversal. Instead, we collect
// instrumentation points and apply them in a second pass.
//
// Why this approach:
//  1. Modifying AST during traversal can cause iteration issues
//  2. We need to insert statements BEFORE access statements
//  3. Inserting nodes while walking can invalidate positions
//
// Two-Pass Algorithm:
//
//	Pass 1 (Visit): Identify all memory accesses, record their locations
//	Pass 2 (Apply): Insert race detection calls at recorded locations
//
// Future Optimization (Phase 6B):
// When integrating with Go compiler, we can leverage compiler's instrumentation
// infrastructure which handles AST modification more elegantly.
type instrumentVisitor struct {
	// fset is the file set for source position information.
	// Used for error messages and debugging.
	fset *token.FileSet

	// file is the AST file being instrumented.
	file *ast.File

	// instrumentationPoints records where to insert race calls.
	// Key: ast.Node (the access node)
	// Value: instrumentPoint (details about the instrumentation)
	instrumentationPoints []instrumentPoint

	// stats tracks instrumentation statistics.
	// Exported via GetStats() for reporting.
	stats InstrumentStats

	// currentFunc tracks the current function declaration for context-aware filtering.
	// v0.7.2: Used to skip named return values and function parameters (stack-local).
	currentFunc *ast.FuncDecl

	// currentFuncLit tracks the current function literal (closure) for context-aware filtering.
	// v0.7.2: Used to skip named return values and function parameters in closures.
	currentFuncLit *ast.FuncLit

	// escapeInfo holds compiler-derived escape analysis results.
	// v0.8.0: If a variable doesn't escape to heap, it's stack-local and cannot race.
	// This is optional - if nil, we skip escape-based filtering.
	escapeInfo *EscapeInfo

	// filename is the relative filename for escape analysis lookups.
	// Must match the format in EscapeInfo (e.g., "main.go" not "./main.go").
	filename string
}

// InstrumentPoint represents a location where race detection should be inserted.
// Exported for testing purposes.
//
//nolint:revive // InstrumentPoint is a clear, descriptive name for this type
type InstrumentPoint struct {
	// Node is the AST node performing the memory access.
	// Example: *ast.AssignStmt, *ast.UnaryExpr (dereference)
	Node ast.Node

	// AccessType indicates whether this is a read or write.
	AccessType AccessType

	// Addr is the expression representing the memory address.
	// Example: &x, ptr, &arr[0], &obj.field
	Addr ast.Expr
}

// AccessType classifies memory access operations.
// Exported for testing purposes.
type AccessType int

const (
	// AccessRead indicates a memory read operation.
	AccessRead AccessType = iota
	// AccessWrite indicates a memory write operation.
	AccessWrite
)

// instrumentPoint is the internal type (lowercase for private use).
type instrumentPoint = InstrumentPoint

// isNamedReturn checks if the identifier is a named return value.
// v0.7.2: Named return values are stack-local and cannot have data races.
//
// Example:
//
//	func foo() (result int, err error) {
//	    result = 42  // result is a named return value - skip instrumentation
//	    return
//	}
func (v *instrumentVisitor) isNamedReturn(ident *ast.Ident) bool {
	// Check current function declaration
	if v.currentFunc != nil && v.currentFunc.Type.Results != nil {
		for _, field := range v.currentFunc.Type.Results.List {
			for _, name := range field.Names {
				if name.Name == ident.Name {
					return true
				}
			}
		}
	}

	// Check current function literal (closure)
	if v.currentFuncLit != nil && v.currentFuncLit.Type.Results != nil {
		for _, field := range v.currentFuncLit.Type.Results.List {
			for _, name := range field.Names {
				if name.Name == ident.Name {
					return true
				}
			}
		}
	}

	return false
}

// isParameter checks if the identifier is a function parameter.
// v0.7.2: Function parameters (pass-by-value) are stack-local copies
// and cannot have data races with other goroutines.
//
// Example:
//
//	func process(x int) {
//	    x = x + 1  // x is a parameter copy - skip instrumentation
//	}
func (v *instrumentVisitor) isParameter(ident *ast.Ident) bool {
	// Check current function declaration
	if v.currentFunc != nil && v.currentFunc.Type.Params != nil {
		for _, field := range v.currentFunc.Type.Params.List {
			for _, name := range field.Names {
				if name.Name == ident.Name {
					return true
				}
			}
		}
	}

	// Check current function literal (closure)
	if v.currentFuncLit != nil && v.currentFuncLit.Type.Params != nil {
		for _, field := range v.currentFuncLit.Type.Params.List {
			for _, name := range field.Names {
				if name.Name == ident.Name {
					return true
				}
			}
		}
	}

	return false
}

// StackLocalReason indicates why a variable is considered stack-local.
// v0.8.0: Used for tracking skip reasons in statistics.
type StackLocalReason int

const (
	// NotStackLocal indicates the variable may escape and needs instrumentation.
	NotStackLocal StackLocalReason = iota
	// StackLocalNamedReturn indicates it's a named return value.
	StackLocalNamedReturn
	// StackLocalParameter indicates it's a function parameter.
	StackLocalParameter
	// StackLocalEscapeAnalysis indicates escape analysis says it doesn't escape.
	StackLocalEscapeAnalysis
)

// isStackLocal checks if the identifier is known to be stack-local.
// v0.7.2: Stack-local variables cannot have data races because each
// goroutine has its own stack.
//
// Currently detected stack-local patterns:
// - Named return values (result in `func() (result int)`)
// - Function parameters (x in `func(x int)`)
// - v0.8.0: Variables that don't escape to heap (from compiler escape analysis)
//
// Future: Add detection for loop variables and short-lived locals.
//
//nolint:unused // Reserved for public API - convenience wrapper
func (v *instrumentVisitor) isStackLocal(ident *ast.Ident) bool {
	return v.isStackLocalWithReason(ident) != NotStackLocal
}

// isStackLocalWithReason checks if the identifier is stack-local and returns the reason.
// v0.8.0: Added for tracking skip reasons in statistics.
func (v *instrumentVisitor) isStackLocalWithReason(ident *ast.Ident) StackLocalReason {
	// Check AST-based patterns first (cheap)
	if v.isNamedReturn(ident) {
		return StackLocalNamedReturn
	}
	if v.isParameter(ident) {
		return StackLocalParameter
	}

	// v0.8.0: Check compiler-derived escape analysis
	// If escape info is available and variable doesn't escape, it's stack-local
	if v.escapeInfo != nil && ident.Pos().IsValid() {
		pos := v.fset.Position(ident.Pos())
		// If the variable at this location DOES escape, we must instrument it
		// If it does NOT escape, it's stack-local and we can skip
		if !v.escapeInfo.DoesEscape(pos.Filename, pos.Line, pos.Column) {
			// Variable doesn't escape to heap - stack-local, skip instrumentation
			return StackLocalEscapeAnalysis
		}
	}

	return NotStackLocal
}

// Visit implements ast.Visitor interface.
//
// This method is called by ast.Walk() for each node in the AST.
// We inspect the node type and record instrumentation points for
// memory access operations.
//
// Nodes we care about (MVP scope):
//  1. *ast.AssignStmt: Variable assignments (x = 42)
//  2. *ast.UnaryExpr (MUL): Pointer dereferences (*ptr)
//  3. *ast.IndexExpr: Array/slice accesses (arr[0])
//  4. *ast.SelectorExpr: Struct field accesses (obj.field)
//
// Nodes we skip (for now):
//   - Function calls: race detection happens inside callee
//   - Constants: no memory access
//   - Type declarations: no runtime access
//
// Parameters:
//   - node: AST node being visited
//
// Returns:
//   - ast.Visitor: Next visitor (self to continue, nil to stop)
//
// Algorithm:
//
//	For each node type, determine if it's a memory access.
//	If yes, record an instrumentPoint with:
//	  - The node itself
//	  - Access type (read vs write)
//	  - Address expression
//
// Thread Safety: NOT thread-safe (modifies instrumentationPoints).
func (v *instrumentVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *ast.FuncDecl:
		// Track current function for context-aware filtering.
		// v0.7.2: Used to skip named return values and parameters (stack-local).
		prevFunc := v.currentFunc
		v.currentFunc = n
		ast.Walk(v, n.Body)
		v.currentFunc = prevFunc
		return nil // Already walked the body

	case *ast.FuncLit:
		// Track current function literal (closure) for context-aware filtering.
		// v0.7.2: Used to skip named return values and parameters in closures.
		prevFuncLit := v.currentFuncLit
		v.currentFuncLit = n
		ast.Walk(v, n.Body)
		v.currentFuncLit = prevFuncLit
		return nil // Already walked the body

	case *ast.AssignStmt:
		// Assignment: x = 42, *ptr = 42, arr[0] = 42
		// These are WRITE operations on the left-hand side.
		v.visitAssignment(n)

	case *ast.IncDecStmt:
		// Increment/decrement: i++, i--, counter++, counter--
		// These are both READ and WRITE operations on the same variable.
		// Example: counter++ is equivalent to counter = counter + 1
		v.visitIncDec(n)

	case *ast.UnaryExpr:
		// Dereference: *ptr (in some contexts)
		// Can be either read or write depending on context.
		// For MVP, we'll instrument as READ (simpler).
		// Future: Context-aware detection (read vs write).
		if n.Op == token.MUL {
			v.visitDereference(n)
		}

	case *ast.StarExpr:
		// Pointer dereference: *ptr
		// Go parser uses StarExpr for pointer dereference in most contexts
		// (e.g., *ptr++ becomes IncDecStmt with X = StarExpr)
		// v0.8.2: Fix for Issue #27 - must instrument pointer dereferences
		v.visitStarExpr(n)

	case *ast.IndexExpr:
		// Array/slice access: arr[0], slice[i]
		// Context determines read vs write.
		// For MVP, we'll instrument as READ.
		v.visitIndexAccess(n)

	case *ast.SelectorExpr:
		// Struct field access: obj.field, ptr.field
		// Context determines read vs write.
		// For MVP, we'll instrument as READ.
		v.visitFieldAccess(n)
	}

	// Continue walking the AST.
	return v
}

// visitAssignment handles assignment statements: x = 42, *ptr = 42, arr[0] = 42.
//
// Algorithm:
//  1. For each right-hand side expression (RHS), record READ accesses
//  2. For each left-hand side expression (LHS), record WRITE accesses
//  3. Extract the address expression (&x, ptr, &arr[0], etc.)
//  4. Add instrumentPoint to record
//
// Examples:
//
//	x = 42           → RaceWrite(&x)
//	x = y            → RaceRead(&y), RaceWrite(&x)
//	*ptr = 42        → RaceWrite(ptr)
//	arr[0] = 42      → RaceWrite(&arr[0])
//	obj.field = 42   → RaceWrite(&obj.field)
//
// Special case - variable declarations (x := y):
//
//	For ":=" assignments, LHS is being DECLARED, not modified.
//	We only instrument RHS reads, not LHS writes.
//	Example: val := counter  →  RaceRead(&counter), NO RaceWrite for val
//
// Multiple assignments (x, y = 1, 2) are handled by iterating Lhs/Rhs slices.
//
// Parameters:
//   - stmt: Assignment statement node
func (v *instrumentVisitor) visitAssignment(stmt *ast.AssignStmt) {
	// First, instrument RHS reads (right side of =)
	for _, rhs := range stmt.Rhs {
		// Extract variables being READ from RHS
		v.extractReads(rhs, stmt)
	}

	// Then, instrument LHS writes (left side of =)
	// SKIP if this is := (variable declaration) - no write instrumentation needed
	if stmt.Tok == token.DEFINE {
		// This is :=, LHS is being declared, not written
		// Example: val := counter
		// We already instrumented counter read above, no need to instrument val
		return
	}

	// For regular assignment (=), instrument LHS writes
	for _, lhs := range stmt.Lhs {
		// v0.7.2+: Skip stack-local variables (named returns, parameters, non-escaping)
		// These cannot have data races as each goroutine has its own stack.
		if ident, ok := lhs.(*ast.Ident); ok {
			reason := v.isStackLocalWithReason(ident)
			if reason != NotStackLocal {
				// Track why it was skipped
				if reason == StackLocalEscapeAnalysis {
					v.stats.EscapeSkipped++
				} else {
					v.stats.ConstantsSkipped++ // Reuse for named returns/params
				}
				continue
			}
		}

		// Skip if this expression shouldn't be instrumented
		if !shouldInstrument(lhs) {
			v.trackSkipped(lhs)
			continue
		}

		// Extract address expression for this LHS.
		addr := v.extractAddress(lhs)
		if addr == nil {
			// Skip if we can't extract address (e.g., blank identifier _)
			continue
		}

		// Record instrumentation point.
		v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
			Node:       stmt,
			AccessType: AccessWrite,
			Addr:       addr,
		})
		v.stats.WritesInstrumented++
	}
}

// visitIncDec handles increment/decrement statements: i++, i--, counter++, counter--.
//
// These statements are both a READ and WRITE of the same location.
// Example: counter++ is semantically equivalent to counter = counter + 1
//
// Algorithm:
//  1. Extract the address of the operand
//  2. Record a READ instrumentation point (for the read part)
//  3. Record a WRITE instrumentation point (for the write part)
//
// Examples:
//
//	i++           → RaceRead(&i), RaceWrite(&i)
//	counter--     → RaceRead(&counter), RaceWrite(&counter)
//	*ptr++        → RaceRead(ptr), RaceWrite(ptr)
//	arr[0]++      → RaceRead(&arr[0]), RaceWrite(&arr[0])
//
// Parameters:
//   - stmt: IncDecStmt node
func (v *instrumentVisitor) visitIncDec(stmt *ast.IncDecStmt) {
	// v0.8.3: Fix for Issue #30 - handle SelectorExpr specially in IncDecStmt
	// In IncDecStmt context, we KNOW the operand is addressable (otherwise Go won't compile)
	// So we can safely instrument SelectorExpr here even though shouldInstrument skips it
	if sel, ok := stmt.X.(*ast.SelectorExpr); ok {
		// s.x++ - struct field access, must instrument
		// Pass stmt as parent so findParentStatement works correctly
		v.visitSelectorInModifyContext(stmt, sel)
		return
	}

	// Skip if this expression shouldn't be instrumented
	if !shouldInstrument(stmt.X) {
		v.trackSkipped(stmt.X)
		return
	}

	// Extract address of the operand
	addr := v.extractAddress(stmt.X)
	if addr == nil {
		return
	}

	// Record READ (the value is read before increment/decrement)
	v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
		Node:       stmt,
		AccessType: AccessRead,
		Addr:       addr,
	})
	v.stats.ReadsInstrumented++

	// Record WRITE (the new value is written back)
	// Use a fresh address expression to avoid AST sharing issues
	addrWrite := v.extractAddress(stmt.X)
	if addrWrite != nil {
		v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
			Node:       stmt,
			AccessType: AccessWrite,
			Addr:       addrWrite,
		})
		v.stats.WritesInstrumented++
	}
}

// extractReads extracts read operations from an expression.
//
// This helper finds all variable/field/array reads in an expression
// and records them for instrumentation.
//
// Examples:
//
//	counter        → RaceRead(&counter)
//	x + y          → RaceRead(&x), RaceRead(&y)
//	arr[i]         → RaceRead(&arr[i]), RaceRead(&i)
//	obj.field      → RaceRead(&obj.field)
//
// Parameters:
//   - expr: Expression to analyze
//   - stmt: Parent statement (for instrumentation point)
func (v *instrumentVisitor) extractReads(expr ast.Expr, stmt ast.Stmt) {
	// Walk the expression and find all identifiers/selectors/indexes
	ast.Inspect(expr, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.Ident:
			// Simple variable read: counter
			// v0.7.2+: Skip stack-local variables (named returns, parameters, non-escaping)
			reason := v.isStackLocalWithReason(e)
			if reason != NotStackLocal {
				// Track why it was skipped
				if reason == StackLocalEscapeAnalysis {
					v.stats.EscapeSkipped++
				}
				return true // Skip but continue walking
			}
			// Skip if this expression shouldn't be instrumented
			if !shouldInstrument(e) {
				v.trackSkipped(e)
				return true
			}
			// Create address expression
			addr := &ast.UnaryExpr{Op: token.AND, X: e}
			v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
				Node:       stmt,
				AccessType: AccessRead,
				Addr:       addr,
			})
			v.stats.ReadsInstrumented++

		case *ast.SelectorExpr:
			// Struct field read: obj.field (e.g., os.Args, person.Name)
			// IMPORTANT: Return false to stop walking into children (X and Sel)
			// Otherwise we'd instrument both &os.Args AND &os AND &Args separately!
			if !shouldInstrument(e) {
				v.trackSkipped(e)
				return false // Don't walk into children
			}
			addr := &ast.UnaryExpr{Op: token.AND, X: e}
			v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
				Node:       stmt,
				AccessType: AccessRead,
				Addr:       addr,
			})
			v.stats.ReadsInstrumented++
			return false // Don't walk into X (os) and Sel (Args) separately

		case *ast.IndexExpr:
			// Array/slice read: arr[i]
			if !shouldInstrument(e) {
				v.trackSkipped(e)
				return true
			}
			addr := &ast.UnaryExpr{Op: token.AND, X: e}
			v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
				Node:       stmt,
				AccessType: AccessRead,
				Addr:       addr,
			})
			v.stats.ReadsInstrumented++

		case *ast.UnaryExpr:
			if e.Op == token.MUL {
				// Pointer dereference: *ptr (UnaryExpr form)
				if !shouldInstrument(e) {
					v.trackSkipped(e)
					return true
				}
				addr := e.X // ptr itself is the address
				v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
					Node:       stmt,
					AccessType: AccessRead,
					Addr:       addr,
				})
				v.stats.ReadsInstrumented++
			}

		case *ast.StarExpr:
			// Pointer dereference: *ptr (StarExpr form)
			// v0.8.2: Fix for Issue #27 - always instrument pointer dereferences
			addr := e.X // ptr itself is the address
			v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
				Node:       stmt,
				AccessType: AccessRead,
				Addr:       addr,
			})
			v.stats.ReadsInstrumented++

		case *ast.KeyValueExpr:
			// Struct literal field: Point{X: 1, Y: 2}
			// Key is the field name (not a variable), Value is the actual expression
			// Don't walk into Key (field name), only walk into Value
			v.extractReads(e.Value, stmt)
			return false // Don't continue walking - we handled it

		case *ast.CompositeLit:
			// Composite literal: Type{...} or []Type{...}
			// Skip Type (it's a type name, not addressable), only walk into Elts
			for _, elt := range e.Elts {
				v.extractReads(elt, stmt)
			}
			return false // Don't continue walking - we handled it

		case *ast.CallExpr:
			// Function call or type conversion: func(args) or Type(expr)
			// Skip Fun (could be function, method, or type - none are addressable)
			// For make/new, first arg is a type - skip it; only process remaining args
			if ident, ok := e.Fun.(*ast.Ident); ok && (ident.Name == "make" || ident.Name == "new") {
				// make(Type, size) or new(Type) - skip first arg (type), process rest
				for i := 1; i < len(e.Args); i++ {
					v.extractReads(e.Args[i], stmt)
				}
			} else {
				// Regular function call - process all args
				for _, arg := range e.Args {
					v.extractReads(arg, stmt)
				}
			}
			return false // Don't continue walking - we handled it

		// Type expressions - skip entirely (types are not addressable)
		case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.StructType,
			*ast.InterfaceType, *ast.FuncType, *ast.Ellipsis:
			// These are type definitions, not value expressions
			return false

		case *ast.IndexListExpr:
			// Generic instantiation: Func[T, U](args)
			// Skip entirely - cannot take address of generic function
			return false

		case *ast.TypeAssertExpr:
			// Type assertion: x.(Type)
			// Skip Type (it's a type name), only walk into X
			v.extractReads(e.X, stmt)
			return false // Don't continue walking - we handled it

		case *ast.FuncLit:
			// Anonymous function: func() { ... }
			// Don't walk into function body - separate scope
			return false
		}
		return true
	})
}

// isBuiltinIdent returns true if the identifier is a built-in (no instrumentation needed).
//
// This includes:
//   - Built-in constants: nil, true, false, iota
//   - Built-in functions: make, new, len, cap, append, copy, delete, close, panic, recover, etc.
//   - Built-in types: int, string, byte, error, etc.
//
// You cannot take the address of built-in functions or types, so they must be excluded.
func isBuiltinIdent(name string) bool {
	builtins := map[string]bool{
		// Built-in constants
		"nil":   true,
		"true":  true,
		"false": true,
		"iota":  true,
		// Built-in functions (cannot take address)
		"make":    true,
		"new":     true,
		"len":     true,
		"cap":     true,
		"append":  true,
		"copy":    true,
		"delete":  true,
		"close":   true,
		"panic":   true,
		"recover": true,
		"print":   true,
		"println": true,
		"complex": true,
		"real":    true,
		"imag":    true,
		"clear":   true,
		"min":     true,
		"max":     true,
		// Built-in types (cannot take address)
		"bool":       true,
		"byte":       true,
		"complex64":  true,
		"complex128": true,
		"error":      true,
		"float32":    true,
		"float64":    true,
		"int":        true,
		"int8":       true,
		"int16":      true,
		"int32":      true,
		"int64":      true,
		"rune":       true,
		"string":     true,
		"uint":       true,
		"uint8":      true,
		"uint16":     true,
		"uint32":     true,
		"uint64":     true,
		"uintptr":    true,
		"any":        true,
		"comparable": true,
	}
	return builtins[name]
}

// shouldInstrument determines if an expression should be instrumented.
//
// This function filters out expressions that don't need race detection:
//   - Constants (const X = 42) - values never change at runtime
//   - Built-in identifiers (nil, true, false, iota) - language-level constants
//   - Built-in functions (make, new, len, etc.) - cannot take address
//   - Built-in types (int, string, byte, etc.) - cannot take address
//   - Function identifiers (func foo()) - cannot take address of function value
//   - Package identifiers (os, fmt) - cannot take address of package
//   - Type identifiers (type MyType) - cannot take address of type
//   - Literals (42, "hello", 3.14) - compile-time values
//   - Blank identifier (_) - special identifier for discarding values
//
// Rationale:
// Race conditions occur when multiple goroutines access the SAME memory
// location and at least one access is a write. Constants and literals don't
// occupy mutable memory, so they cannot participate in data races.
// Functions, types, and packages are not addressable in Go.
//
// Example:
//
//	const X = 42       // Never changes, skip instrumentation
//	y := X + 1         // X read is safe (constant), y write needs instrumentation
//	z := 42            // 42 is literal, skip instrumentation
//	_ = z              // Blank identifier, skip instrumentation
//	f := parseSpec     // Function reference, skip (cannot take address)
//	s := make([]int, 10) // Built-in make, skip
//	str := string(data)  // Type conversion, skip
//
// Performance Impact:
// Reduces instrumentation overhead by eliminating unnecessary race calls
// on constant values. Expected reduction: 5-15% of total instrumentation points.
//
// Parameters:
//   - expr: Expression to check
//
// Returns:
//   - bool: true if expression needs instrumentation, false otherwise
//
// Thread Safety: Read-only, safe for concurrent use.
func shouldInstrument(expr ast.Expr) bool {
	// Skip constants
	if isConstant(expr) {
		return false
	}

	// Skip built-in identifiers, functions, types, packages, and blank identifier
	if ident, ok := expr.(*ast.Ident); ok {
		// Blank identifier _ (used to discard values)
		if ident.Name == "_" {
			return false
		}
		// Built-in identifiers, functions, and types
		if isBuiltinIdent(ident.Name) {
			return false
		}

		// ULTRA-CONSERVATIVE: Only instrument if we're 100% sure it's a variable
		// Without full type checking, ident.Obj is often nil for:
		// - Functions defined in other files
		// - Types defined in other packages
		// - Package-level variables from other files
		// - Generic type parameters
		//
		// We can only safely instrument when:
		// 1. ident.Obj is not nil (parser resolved the symbol)
		// 2. ident.Obj.Kind is ast.Var (it's definitely a variable)
		//
		// This may miss some race conditions, but avoids compilation errors.
		if ident.Obj == nil {
			// Cannot determine kind - skip to be safe
			// This skips functions/types from other files/packages
			return false
		}

		// Only instrument variables, skip everything else
		if ident.Obj.Kind != ast.Var {
			return false
		}
	}

	// Skip ALL SelectorExpr - too many non-addressable cases without type info:
	// - Package functions: os.ReadFile, strconv.Atoi
	// - Method values: obj.Method
	// - Fields on return values: GetObj().Field (not addressable)
	// - Method calls: result.String()
	//
	// Without full type information, we cannot reliably distinguish:
	// - obj.Field (addressable if obj is a variable)
	// - GetObj().Field (NOT addressable - return value)
	// - obj.Method (NOT addressable - method value)
	//
	// Conservative approach: skip ALL SelectorExpr to avoid compilation errors.
	// This may miss some race conditions on struct fields, but ensures safety.
	if _, ok := expr.(*ast.SelectorExpr); ok {
		return false
	}

	// Skip IndexExpr on maps - cannot take address of map element
	// Without type info, we cannot distinguish map[key] from slice[i]
	// Conservative approach: skip all IndexExpr to avoid "cannot take address of" errors
	// This may miss some race conditions on slice/array elements, but it's safer
	if _, ok := expr.(*ast.IndexExpr); ok {
		// TODO: With type info, we could distinguish maps from slices/arrays
		// For now, skip all to avoid compilation errors
		return false
	}

	// Skip literals
	if isLiteral(expr) {
		return false
	}

	return true
}

// trackSkipped tracks why an expression was skipped (for statistics).
//
// This helper method classifies skipped expressions and increments
// the appropriate statistic counter.
//
// Parameters:
//   - expr: Expression that was skipped
//
// Thread Safety: NOT thread-safe (modifies visitor stats).
func (v *instrumentVisitor) trackSkipped(expr ast.Expr) {
	if isConstant(expr) {
		v.stats.ConstantsSkipped++
		return
	}

	if ident, ok := expr.(*ast.Ident); ok {
		if ident.Name == "_" {
			v.stats.BlanksSkipped++
			return
		}
		if isBuiltinIdent(ident.Name) {
			v.stats.BuiltinsSkipped++
			return
		}
	}

	if isLiteral(expr) {
		v.stats.LiteralsSkipped++
		return
	}
}

// isConstant checks if expression is a constant declared with 'const'.
//
// Go's type system tracks whether an identifier refers to a constant
// via the Obj (object) field. Constants are declared with 'const' keyword
// and their values are immutable.
//
// Example:
//
//	const X = 42        // isConstant(&ast.Ident{Name:"X"}) → true
//	var Y = 42          // isConstant(&ast.Ident{Name:"Y"}) → false
//
// Limitations:
// For MVP, we rely on ast.Ident.Obj.Kind == ast.Con. This works for
// identifiers in the same file. Cross-package constants might not have
// Obj populated (depends on type checking).
//
// Future Enhancement (Phase 6B):
// Use go/types package for full type information and cross-package constants.
//
// Parameters:
//   - expr: Expression to check
//
// Returns:
//   - bool: true if expression is a declared constant
//
// Thread Safety: Read-only, safe for concurrent use.
func isConstant(expr ast.Expr) bool {
	// Check if it's an identifier declared as const
	if ident, ok := expr.(*ast.Ident); ok {
		if ident.Obj != nil && ident.Obj.Kind == ast.Con {
			return true
		}
	}
	return false
}

// isLiteral checks if expression is a literal value (compile-time constant).
//
// Literals are values written directly in code: numbers, strings, booleans, etc.
// They are stored in the compiled binary's data section, not in mutable memory.
//
// Go defines literals in ast.BasicLit for most primitive types.
//
// Examples:
//
//	42          → *ast.BasicLit{Kind: token.INT}
//	"hello"     → *ast.BasicLit{Kind: token.STRING}
//	3.14        → *ast.BasicLit{Kind: token.FLOAT}
//	true        → Handled by isBuiltinIdent (special case)
//
// Limitations:
// Composite literals ([]int{1,2,3}, struct{}{}) are NOT BasicLit.
// For MVP, we only handle basic literals. Composite literals will still
// be instrumented (conservative approach).
//
// Future Enhancement:
// Add *ast.CompositeLit detection if performance profiling shows benefit.
//
// Parameters:
//   - expr: Expression to check
//
// Returns:
//   - bool: true if expression is a literal value
//
// Thread Safety: Read-only, safe for concurrent use.
func isLiteral(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.BasicLit: // Covers: int, float, string, char, imaginary literals
		return true
	}
	return false
}

// visitDereference handles pointer dereferences: *ptr (UnaryExpr form).
//
// Dereferences can be either reads or writes depending on context:
//
//	y := *ptr   → READ
//	*ptr = 42   → WRITE
//
// For MVP, we'll conservatively instrument all dereferences as READS.
// The WRITE case is handled separately in visitAssignment.
//
// Future Enhancement (Phase 6B):
// Perform context analysis to determine read vs write accurately.
//
// Parameters:
//   - expr: Unary expression node (dereference)
func (v *instrumentVisitor) visitDereference(expr *ast.UnaryExpr) {
	// The operand of * is the pointer being dereferenced.
	// Example: *ptr → operand is ptr
	addr := expr.X

	// Record instrumentation point.
	v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
		Node:       expr,
		AccessType: AccessRead,
		Addr:       addr,
	})
}

// visitStarExpr handles pointer dereferences: *ptr (StarExpr form).
//
// Go parser uses StarExpr for pointer dereference in most contexts,
// especially in statements like *ptr++ (IncDecStmt with X = StarExpr).
//
// v0.8.2: Fix for Issue #27 - pointer dereferences must always be instrumented
// regardless of whether the pointer variable itself escapes to heap.
// The memory being accessed through the pointer CAN be shared.
//
// Example:
//
//	func update(ptr *int) {  // ptr doesn't escape (stack-local copy)
//	    *ptr++               // But *ptr accesses potentially shared memory!
//	}
//
// Parameters:
//   - expr: Star expression node (pointer dereference)
func (v *instrumentVisitor) visitStarExpr(expr *ast.StarExpr) {
	// The operand of * is the pointer being dereferenced.
	// Example: *ptr → operand is ptr
	addr := expr.X

	// Record instrumentation point.
	// Pointer dereferences MUST be instrumented - the memory they access
	// can be shared even if the pointer variable itself is stack-local.
	v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
		Node:       expr,
		AccessType: AccessRead,
		Addr:       addr,
	})
	v.stats.ReadsInstrumented++
}

// visitSelectorInModifyContext handles struct field access in modify contexts (IncDecStmt, AssignStmt LHS).
//
// v0.8.3: Fix for Issue #30 - in modify contexts we KNOW the selector is addressable
// (otherwise Go wouldn't compile), so we can safely instrument it.
//
// Examples:
//
//	s.x++         → RaceRead(&s.x), RaceWrite(&s.x)
//	s.field = 42  → RaceWrite(&s.field)
//
// Parameters:
//   - parentStmt: The parent statement (IncDecStmt or AssignStmt) - used as Node for instrumentation
//   - expr: SelectorExpr being modified
//
// IMPORTANT: We use parentStmt as Node (not expr) because findParentStatement()
// needs a Stmt to work correctly. If we pass SelectorExpr, findParentStatement
// returns the outermost containing statement (BlockStmt), not the IncDecStmt.
func (v *instrumentVisitor) visitSelectorInModifyContext(parentStmt ast.Stmt, expr *ast.SelectorExpr) {
	// For s.x, we need to take address of the field: &s.x
	// Create UnaryExpr with & operator wrapping the SelectorExpr
	addrExpr := &ast.UnaryExpr{
		Op: token.AND,
		X:  expr,
	}

	// Record READ (value is read before modification in ++ case)
	// Use parentStmt as Node so findParentStatement returns correct statement
	v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
		Node:       parentStmt,
		AccessType: AccessRead,
		Addr:       addrExpr,
	})
	v.stats.ReadsInstrumented++

	// Record WRITE (new value is written back)
	// Create a separate address expression to avoid AST sharing
	addrExprWrite := &ast.UnaryExpr{
		Op: token.AND,
		X:  expr,
	}
	v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
		Node:       parentStmt,
		AccessType: AccessWrite,
		Addr:       addrExprWrite,
	})
	v.stats.WritesInstrumented++
}

// visitIndexAccess handles array/slice accesses: arr[0], slice[i].
//
// Similar to dereferences, index accesses can be reads or writes:
//
//	y := arr[0]   → READ
//	arr[0] = 42   → WRITE
//
// For MVP, we'll instrument as READS. WRITE case handled in visitAssignment.
//
// Parameters:
//   - expr: Index expression node
func (v *instrumentVisitor) visitIndexAccess(expr *ast.IndexExpr) {
	// Index access: arr[index]
	// Address is &arr[index] conceptually, but we need arr base address.
	// For simplicity, we'll use the entire IndexExpr as the address.
	addr := expr

	v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
		Node:       expr,
		AccessType: AccessRead,
		Addr:       addr,
	})
}

// visitFieldAccess handles struct field accesses: obj.field.
//
// Field accesses can be reads or writes:
//
//	y := obj.field   → READ
//	obj.field = 42   → WRITE
//
// For MVP, we'll instrument as READS. WRITE case handled in visitAssignment.
//
// Parameters:
//   - expr: Selector expression node (struct field access)
func (v *instrumentVisitor) visitFieldAccess(expr *ast.SelectorExpr) {
	// Field access: obj.field
	// Address is &obj.field
	addr := expr

	v.instrumentationPoints = append(v.instrumentationPoints, InstrumentPoint{
		Node:       expr,
		AccessType: AccessRead,
		Addr:       addr,
	})
}

// extractAddress extracts the address expression from an LHS expression.
//
// This helper converts different LHS forms into appropriate address expressions
// for race detection calls.
//
// Transformations:
//
//	x           → &x
//	*ptr        → ptr
//	arr[0]      → &arr[0]
//	obj.field   → &obj.field
//	_           → nil (blank identifier, skip instrumentation)
//
// Parameters:
//   - expr: Left-hand side expression
//
// Returns:
//   - ast.Expr: Address expression, or nil if not instrumentable
func (v *instrumentVisitor) extractAddress(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.Ident:
		// Simple variable: x
		// Address: &x
		return &ast.UnaryExpr{
			Op: token.AND,
			X:  e,
		}

	case *ast.UnaryExpr:
		if e.Op == token.MUL {
			// Dereference: *ptr (UnaryExpr form)
			// Address is ptr itself
			return e.X
		}

	case *ast.StarExpr:
		// Dereference: *ptr (StarExpr form)
		// v0.8.2: Fix for Issue #27
		// Address is ptr itself
		return e.X

	case *ast.IndexExpr:
		// Array access: arr[0]
		// Address: &arr[0]
		return &ast.UnaryExpr{
			Op: token.AND,
			X:  e,
		}

	case *ast.SelectorExpr:
		// Field access: obj.field
		// Address: &obj.field
		return &ast.UnaryExpr{
			Op: token.AND,
			X:  e,
		}
	}

	// Unsupported expression type, skip instrumentation.
	return nil
}

// InstrumentOptions configures the instrumentation visitor.
//
// v0.8.0: Added to support compiler-derived escape analysis for
// more accurate stack-local variable detection.
//
//nolint:revive // Stuttering is intentional for API clarity (instrument.InstrumentOptions)
type InstrumentOptions struct {
	// EscapeInfo contains compiler-derived escape analysis results.
	// If nil, escape-based filtering is disabled (conservative approach).
	EscapeInfo *EscapeInfo

	// Filename is the relative filename for escape analysis lookups.
	// Should match the format in EscapeInfo (e.g., "main.go").
	Filename string

	// EnableCoalescing enables BigFoot coalescing optimization.
	// Default: false (disabled for safety).
	EnableCoalescing bool
}

// newInstrumentVisitor creates a new instrumentVisitor instance.
//
// Parameters:
//   - fset: File set for source positions
//   - file: AST file to instrument
//
// Returns:
//   - *instrumentVisitor: New visitor instance
func newInstrumentVisitor(fset *token.FileSet, file *ast.File) *instrumentVisitor {
	return newInstrumentVisitorWithOptions(fset, file, InstrumentOptions{})
}

// newInstrumentVisitorWithOptions creates a new instrumentVisitor with options.
//
// v0.8.0: Added to support compiler-derived escape analysis.
//
// Parameters:
//   - fset: File set for source positions
//   - file: AST file to instrument
//   - opts: Instrumentation options (escape info, coalescing, etc.)
//
// Returns:
//   - *instrumentVisitor: New visitor instance
func newInstrumentVisitorWithOptions(fset *token.FileSet, file *ast.File, opts InstrumentOptions) *instrumentVisitor {
	return &instrumentVisitor{
		fset:                  fset,
		file:                  file,
		instrumentationPoints: make([]instrumentPoint, 0, 100), // Pre-allocate for typical file
		escapeInfo:            opts.EscapeInfo,
		filename:              opts.Filename,
	}
}

// GetInstrumentationPoints returns the collected instrumentation points.
//
// This is exported for testing and debugging purposes. It allows tests
// to verify that the visitor correctly identifies memory access operations
// without requiring full AST modification implementation.
//
// Returns:
//   - []instrumentPoint: List of instrumentation points collected during AST walk
//
// Thread Safety: NOT thread-safe (accesses visitor state).
func (v *instrumentVisitor) GetInstrumentationPoints() []instrumentPoint {
	return v.instrumentationPoints
}

// GetStats returns the collected instrumentation statistics.
//
// This is exported for reporting purposes. It allows the build command
// to display detailed statistics about what was instrumented.
//
// Returns:
//   - InstrumentStats: Statistics collected during AST walk
//
// Thread Safety: NOT thread-safe (accesses visitor state).
func (v *instrumentVisitor) GetStats() InstrumentStats {
	return v.stats
}

// ApplyCoalescing applies BigFoot coalescing optimization to reduce barriers.
//
// This method analyzes instrumentation points and groups consecutive operations
// on the same variable. Instead of inserting a barrier BEFORE each operation,
// we insert a SINGLE barrier AFTER the last operation in each group.
//
// Academic Foundation: BigFoot algorithm (PLDI 2017)
// Expected Impact: 40-60% reduction in race barriers
//
// Safety: Conservative - only coalesces when proven safe (same variable,
// consecutive statements, no control flow, no function calls).
//
// Parameters:
//   - enableCoalescing: If false, skip coalescing (debugging mode)
//
// Returns:
//   - CoalescingStats: Statistics about coalescing effectiveness
//
// Thread Safety: NOT thread-safe (modifies instrumentationPoints).
func (v *instrumentVisitor) ApplyCoalescing(enableCoalescing bool) CoalescingStats {
	if !enableCoalescing || len(v.instrumentationPoints) < 2 {
		// No coalescing - return empty stats
		return CoalescingStats{
			TotalOperations: len(v.instrumentationPoints),
		}
	}

	// Analyze instrumentation points for coalescing opportunities
	analyzer := NewCoalescingAnalyzer()
	groups, stats := analyzer.AnalyzeInstrumentationPoints(v.instrumentationPoints, v.file)

	if len(groups) == 0 {
		// No coalescing opportunities found
		return stats
	}

	// Apply coalescing by modifying instrumentation points
	// Strategy: Remove intermediate barriers, keep only last barrier in each group
	coalescedPoints := v.applyCoalescingToPoints(groups)

	// Replace original instrumentation points with coalesced version
	v.instrumentationPoints = coalescedPoints

	return stats
}

// applyCoalescingToPoints applies coalescing groups to instrumentation points.
//
// Algorithm:
//  1. Create set of statements in coalescing groups
//  2. Keep only LAST operation from each group
//  3. Keep all operations NOT in any group
//
// Example:
//
//	Input: [x=1, x=2, x=3, y=1]
//	Groups: [{Operations: [x=1, x=2, x=3]}]
//	Output: [x=3, y=1]  (removed x=1, x=2 barriers)
//
// Parameters:
//   - groups: Coalescing groups from analyzer
//
// Returns:
//   - []instrumentPoint: Coalesced instrumentation points
//
// Thread Safety: NOT thread-safe (reads instrumentationPoints).
func (v *instrumentVisitor) applyCoalescingToPoints(groups []CoalescingGroup) []instrumentPoint {
	// Build map of statements in coalescing groups
	// Key: ast.Node (statement), Value: true if should be REMOVED
	shouldRemove := make(map[ast.Node]bool)

	for _, group := range groups {
		// Remove barriers for all operations EXCEPT the last one
		for i := 0; i < len(group.Operations)-1; i++ {
			shouldRemove[group.Operations[i]] = true
		}
		// Keep the last operation's barrier (group.BarrierPos)
	}

	// Filter instrumentation points - keep only non-removed operations
	coalescedPoints := make([]instrumentPoint, 0, len(v.instrumentationPoints))

	for _, point := range v.instrumentationPoints {
		if !shouldRemove[point.Node] {
			// Keep this instrumentation point
			coalescedPoints = append(coalescedPoints, point)
		}
		// else: Remove this barrier (coalesced)
	}

	return coalescedPoints
}

// ApplyInstrumentation inserts race detection calls into the AST.
//
// This function performs the second pass of instrumentation: it takes
// the collected instrumentation points and modifies the AST to insert
// race.RaceRead() or race.RaceWrite() calls BEFORE each memory access.
//
// Algorithm:
//  1. For each instrumentation point, find its parent statement list
//  2. Create a race detection call expression
//  3. Insert the call BEFORE the original statement
//  4. Handle special cases (assignments, expressions, etc.)
//
// Example Transformation:
//
//	// Original AST:
//	counter = val + 1
//
//	// Instrumented AST:
//	race.RaceWrite(uintptr(unsafe.Pointer(&counter)))
//	counter = val + 1
//
// Parameters:
//   - None (operates on v.instrumentationPoints)
//
// Returns:
//   - error: Instrumentation error, or nil on success
//
// Thread Safety: NOT thread-safe (modifies AST in place).
func (v *instrumentVisitor) ApplyInstrumentation() error {
	// Group instrumentation points by parent statement.
	// We need to insert race calls BEFORE the statement containing the access.
	// Multiple accesses in one statement (e.g., x = y + z) need one call each.

	// For MVP, we'll use a simple approach:
	// Walk the AST again and insert calls directly into statement lists.

	// Create a map of statements to instrument
	stmtToPoints := make(map[ast.Stmt][]instrumentPoint)

	// Find parent statements for each instrumentation point
	for _, point := range v.instrumentationPoints {
		stmt := v.findParentStatement(point.Node)
		if stmt != nil {
			stmtToPoints[stmt] = append(stmtToPoints[stmt], point)
		}
	}

	// Now walk the AST and insert race calls
	ast.Inspect(v.file, func(n ast.Node) bool {
		// Look for statement lists (BlockStmt, CaseClause, etc.)
		switch block := n.(type) {
		case *ast.BlockStmt:
			// Process statements in this block
			newStmts := make([]ast.Stmt, 0, len(block.List)*2)
			for _, stmt := range block.List {
				// Check if this statement needs instrumentation
				if points, ok := stmtToPoints[stmt]; ok {
					// Insert race calls BEFORE this statement
					for _, point := range points {
						raceCall := v.createRaceCall(point)
						if raceCall != nil {
							newStmts = append(newStmts, raceCall)
						}
					}
				}
				// Keep original statement
				newStmts = append(newStmts, stmt)
			}
			block.List = newStmts

		case *ast.CaseClause:
			// Handle switch/select case bodies
			newStmts := make([]ast.Stmt, 0, len(block.Body)*2)
			for _, stmt := range block.Body {
				if points, ok := stmtToPoints[stmt]; ok {
					for _, point := range points {
						raceCall := v.createRaceCall(point)
						if raceCall != nil {
							newStmts = append(newStmts, raceCall)
						}
					}
				}
				newStmts = append(newStmts, stmt)
			}
			block.Body = newStmts

		case *ast.CommClause:
			// Handle select communication case bodies
			newStmts := make([]ast.Stmt, 0, len(block.Body)*2)
			for _, stmt := range block.Body {
				if points, ok := stmtToPoints[stmt]; ok {
					for _, point := range points {
						raceCall := v.createRaceCall(point)
						if raceCall != nil {
							newStmts = append(newStmts, raceCall)
						}
					}
				}
				newStmts = append(newStmts, stmt)
			}
			block.Body = newStmts
		}

		return true
	})

	return nil
}

// findParentStatement finds the statement containing the given node.
//
// This helper walks up the AST from a node to find the enclosing statement.
// This is needed because we insert race calls at statement level, not
// expression level.
//
// Parameters:
//   - node: AST node to find parent for
//
// Returns:
//   - ast.Stmt: Parent statement, or nil if not found
func (v *instrumentVisitor) findParentStatement(node ast.Node) ast.Stmt {
	// For assignments, the node itself is the statement
	if stmt, ok := node.(ast.Stmt); ok {
		return stmt
	}

	// For expressions, we need to find the enclosing statement
	// This is tricky without parent pointers, so we'll use a heuristic:
	// Walk the AST and match nodes
	var result ast.Stmt

	ast.Inspect(v.file, func(n ast.Node) bool {
		// Check if this is a statement containing our node
		if stmt, ok := n.(ast.Stmt); ok {
			// Check if our node is inside this statement
			found := false
			ast.Inspect(stmt, func(inner ast.Node) bool {
				if inner == node {
					found = true
					return false
				}
				return true
			})
			if found {
				result = stmt
				return false // Found it, stop searching
			}
		}
		return true
	})

	return result
}

// createRaceCall creates an AST node for a race detection call.
//
// This function generates an expression statement that calls race.RaceRead()
// or race.RaceWrite() with the appropriate address.
//
// Generated Code:
//   - race.RaceWrite(uintptr(unsafe.Pointer(&x)))
//   - race.RaceRead(uintptr(unsafe.Pointer(&x)))
//
// Parameters:
//   - point: Instrumentation point describing the access
//
// Returns:
//   - ast.Stmt: Expression statement containing the race call
func (v *instrumentVisitor) createRaceCall(point instrumentPoint) ast.Stmt {
	// Determine function name based on access type
	var funcName string
	if point.AccessType == AccessWrite {
		funcName = "RaceWrite"
	} else {
		funcName = "RaceRead"
	}

	// Create the call: race.RaceWrite(uintptr(unsafe.Pointer(&x)))
	// Structure:
	//   race.RaceWrite(
	//     uintptr(
	//       unsafe.Pointer(
	//         &x
	//       )
	//     )
	//   )

	// Build from inside out:

	// 1. Address expression: &x (point.Addr already contains this)
	addrExpr := point.Addr

	// 2. unsafe.Pointer(&x)
	unsafePointerCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("unsafe"),
			Sel: ast.NewIdent("Pointer"),
		},
		Args: []ast.Expr{addrExpr},
	}

	// 3. uintptr(unsafe.Pointer(&x))
	uintptrConversion := &ast.CallExpr{
		Fun:  ast.NewIdent("uintptr"),
		Args: []ast.Expr{unsafePointerCall},
	}

	// 4. race.RaceWrite(...) or race.RaceRead(...)
	raceCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(RacePackageAlias),
			Sel: ast.NewIdent(funcName),
		},
		Args: []ast.Expr{uintptrConversion},
	}

	// 5. Wrap in expression statement
	return &ast.ExprStmt{
		X: raceCall,
	}
}
