# Pure-Go Race Detector - Development Roadmap

> **Strategic Advantage**: Proven FastTrack algorithm implementation without CGO dependency!
> **Approach**: Scientific algorithm + Go best practices - eliminates C++ ThreadSanitizer dependency

**Last Updated**: 2025-12-19 | **Current Version**: v0.8.3 (STABLE) | **Strategy**: MVP → Optimization + Hardening → Advanced Optimizations → Assembly GID → Test Suite → Escape Analysis → Community Bug Fixes → Runtime Integration → Go Proposal | **Milestone**: v0.8.3 (Bug Fixes) → v0.9.0 (Polish) → v1.0.0 (Q1 2026)

---

## 🎯 Vision

Build a **production-ready, pure-Go race detector** that eliminates CGO dependency for race detection, enabling `CGO_ENABLED=0` builds with full race detection capabilities.

### Key Advantages

✅ **FastTrack Algorithm**
- Academic paper (PLDI 2009) provides proven foundation
- 30+ citations and production validation
- Efficient happens-before tracking
- Adaptive epoch ↔ vector clock optimization (64x memory savings)

✅ **Pure Go Implementation**
- No CGO dependency (unlike Go's official `-race` flag)
- Works with `CGO_ENABLED=0` builds
- Cross-platform without C++ compiler requirements
- Easier deployment and distribution

✅ **Standalone Tool**
- AST-based instrumentation (no compiler modification)
- Drop-in replacement for `go build` and `go run`
- Works with existing Go projects immediately
- Community adoption without Go runtime changes

---

## 🚀 Version Strategy

### Philosophy: MVP → Refinement → Integration → Community Validation → Stable

```
v0.1.0-alpha (MVP) ✅ SKIPPED (superseded same day)
         ↓ (same day)
v0.1.0 (FIRST WORKING RELEASE) ✅ RELEASED 2025-11-19
         ↓ (detector works, catches real races!)
v0.2.0 (Performance + Hardening) ✅ RELEASED 2025-11-20
         ↓ (99% overhead reduction, 74× speedup, production-grade!)
v0.3.0 (Advanced Optimizations) ✅ RELEASED 2025-11-28
         ↓ (43× faster VectorClocks, 8× memory reduction, sampling!)
v0.3.2 (Go 1.24+ & Bug Fixes) ✅ RELEASED 2025-12-01
         ↓ (Go 1.24+ requirement, replace directive fix)
v0.4.0-v0.4.10 (Test Command) ✅ RELEASED 2025-12-09/10
         ↓ (racedetector test command, 10 hotfixes)
v0.5.0-v0.5.2 (Assembly GID) ✅ RELEASED 2025-12-10
         ↓ (~2200× faster goroutine ID extraction!)
v0.6.0 (Go Race Test Suite) ✅ RELEASED 2025-12-11
         ↓ (359 test scenarios, 100% pass rate!)
v0.7.0-v0.7.2 (Stack Traces & Hotfixes) ✅ RELEASED 2025-12-18/19
         ↓ (Performance optimizations, false positive fixes)
v0.8.0 (Escape Analysis) ✅ RELEASED 2025-12-19
         ↓ (30-50% fewer false positives, toolexec command!)
v0.8.1 (Escape Analysis Fixes) ✅ RELEASED 2025-12-19
         ↓ (Minor fixes, path normalization)
v0.8.2 (Issue #27: *ptr++ fix) ✅ RELEASED 2025-12-19
         ↓ (Pointer dereference instrumentation, by @thepudds)
v0.8.3 (Issue #30: s.x++ fix) ✅ RELEASED 2025-12-19
         ↓ (Struct field access instrumentation, by @thepudds)
v0.9.0 (Polish & Stabilization) → Final polish before v1.0
         ↓ (1-2 weeks)
v1.0.0 LTS → Production-ready with Go community adoption (Q1 2026)
```

### Critical Milestones

**v0.1.0** = First working release ✅ RELEASED
- FastTrack algorithm fully implemented
- AST instrumentation working (race calls inserted)
- Detects real data races successfully
- Cross-platform support (Linux, macOS, Windows)
- 22,653 lines production code, 970+ test lines
- 70+ tests passing (100% pass rate)

**v0.2.0** = Performance + Production Hardening ✅ RELEASED
- **Performance (Tasks 1-3)**:
  - CAS-based shadow memory (81.4% faster, 0 allocations)
  - BigFoot static coalescing (90% barrier reduction)
  - SmartTrack ownership tracking (10-20% HB reduction)
  - Combined: 99% overhead reduction, 74× speedup
- **Production Hardening (Tasks 4-6)**:
  - Increase limits: 65K goroutines, 281T operations
  - Overflow detection with 90% warnings
  - Stack depot for complete race reports (both stacks!)

**v0.3.0** = Advanced Performance Optimizations ✅ RELEASED
- **Sparse-aware VectorClocks**:
  - Track maxTID to skip empty slots
  - 43× faster Join (500ns → 11.48ns)
  - 20× faster LessOrEqual (300ns → 14.80ns)
- **Address Compression**:
  - 8-byte alignment for sequential accesses
  - Up to 8× memory reduction
- **Sampling-based Detection**:
  - RACEDETECTOR_SAMPLE_RATE environment variable
  - Configurable overhead for CI/CD workflows
- **Enhanced Read-Shared**:
  - 4 inline slots for delayed VectorClock promotion

**v0.3.2** = Go 1.24+ Requirement & Bug Fixes ✅ RELEASED
- **Go 1.24+ requirement**:
  - Benefit from Swiss Tables maps (+30% faster)
  - Improved sync.Map with concurrent hash-trie
  - 2-3% CPU performance improvements
- **Replace directive bug fix (Issue #6)**:
  - Preserve replace directives from original go.mod
  - Convert relative paths to absolute for temp workspace
  - New dependency: golang.org/x/mod v0.30.0

**v0.4.0-v0.4.10** = Test Command ✅ RELEASED
- **`racedetector test` command**:
  - Drop-in replacement for `go test -race`
  - All `go test` flags supported
  - 10 hotfixes for edge cases discovered in real-world testing
- **Validated on complex projects**:
  - gogpu/wgpu - multi-package module with CGO
  - Self-dogfooding - racedetector can instrument itself

**v0.5.0** = Assembly-Optimized Goroutine ID 🚧 IN DEVELOPMENT
- **Native assembly implementation**:
  - amd64: TLS-based access via `(TLS)` pseudo-register
  - arm64: Dedicated g register (R28)
  - goid offset: 152 bytes (verified for Go 1.23-1.25)
- **Performance breakthrough**:
  - 1.73 ns/op (was 3987 ns/op) - ~2200× speedup!
  - Zero allocations on fast path
- **Zero external dependencies**:
  - Pure Go + native assembly
  - No outrigdev/goid or other libraries
  - Strategic decision for Go runtime proposal acceptance
- **Automatic fallback**:
  - runtime.Stack parsing on unsupported platforms
  - Build constraints: `go1.23 && !go1.26 && (amd64 || arm64)`

**v0.6.0** = Go runtime integration (planned)
- Replace `runtime/race/*.syso` (ThreadSanitizer binaries)
- Integrate with Go compiler's `-race` flag
- Official Go toolchain compatibility testing

**v1.0.0** = Production with Community Adoption
- Proven in real-world projects
- Performance competitive with ThreadSanitizer
- Go proposal submitted and accepted
- Long-term support guarantee

**Why v0.1.0 (not alpha)?**: Detector WORKS end-to-end! Successfully detects real races. Alpha phase would imply "might not work" - but it does! Upgrading to stable release reflects actual functionality.

**Why merge v0.3.0 into v0.2.0?**: First impression matters. Better to release ONE production-ready version with both performance and hardening than split into multiple releases. Community feedback (dvyukov) identified critical MVP limitations that MUST be fixed before runtime integration.

**See**: Phase completion reports in `docs/dev/reports/` for detailed progress (private)

---

## 📊 Current Status (v0.8.3 Stable)

**Phase**: ✅ Community Bug Fixes Complete!
**Detector**: Production-grade with critical bug fixes from @thepudds! ⚡
**AST Instrumentation**: Complete with escape analysis and struct field support! ✅

**What Works**:
- ✅ `racedetector build` command (drop-in for `go build`)
- ✅ `racedetector run` command (drop-in for `go run`)
- ✅ `racedetector test` command (drop-in for `go test -race`)
- ✅ `racedetector toolexec` command (for `go build -toolexec`) **NEW in v0.8.0!**
- ✅ **Escape analysis integration** (30-50% fewer false positives) **NEW in v0.8.0!**
- ✅ **FastTrack algorithm** (write/read race detection)
- ✅ **CAS-based shadow memory** (81.4% faster, 0 allocations)
- ✅ **BigFoot coalescing** (90% barrier reduction)
- ✅ **SmartTrack ownership** (10-20% HB check reduction)
- ✅ **Vector clocks** (happens-before relationships)
- ✅ **Adaptive optimization** (epoch ↔ VectorClock, 64x memory savings)
- ✅ **Sync primitives** (Mutex, Channel, WaitGroup tracking)
- ✅ **Complete race reports** with BOTH current AND previous stack traces!
- ✅ **Stack depot** (deduplication using FNV-1a hash)
- ✅ **Race deduplication** (no report spam)
- ✅ **Overflow detection** (warnings at 90% threshold)
- ✅ **Smart filtering** (skips constants, built-ins, literals)
- ✅ **Assembly GID** (~2200× faster goroutine ID)
- ✅ **359 test scenarios** from Go race detector test suite (100% pass rate)
- ✅ **Professional errors** (file:line:column with suggestions)
- ✅ **Verbose mode** (`-v` flag shows instrumentation stats)

**Example Race Detection (v0.2.0)**:
```bash
$ racedetector build examples/dogfooding/simple_race.go
$ ./simple_race
==================
WARNING: DATA RACE
Write at 0xc00000a0b8 by goroutine 4:
  main.doWork()
      /path/to/main.go:45
  main.worker()
      /path/to/main.go:30

Previous Write at 0xc00000a0b8 by goroutine 3:
  main.doWork()
      /path/to/main.go:45
  main.worker()
      /path/to/main.go:30
==================
```

**Validation**:
- ✅ Detects simple race (10 goroutines → counter=2 instead of 10)
- ✅ No false positives on mutex-protected code
- ✅ No false positives on channel synchronization
- ✅ 670+ tests passing (100% pass rate)
- ✅ Works with `CGO_ENABLED=0`
- ✅ Complete stack traces for both accesses

**Performance (v0.2.0)**:
- Hot path overhead: 2-5% (competitive with ThreadSanitizer's 5-10%)
- Zero allocations (0 B/op)
- Scalable to 65,536+ goroutines
- 281T operations supported (years of runtime)
- 74× speedup vs v0.1.0 (99% overhead reduction)
- 90% barrier reduction through BigFoot coalescing

**History**: See [CHANGELOG.md](CHANGELOG.md) for complete release history

---

## 📅 What's Next

### **v0.8.0 - Escape Analysis Integration** (December 2025) [RELEASED! ✅]

**Goal**: Reduce false positives using compiler escape analysis

**Duration**: 1 day (December 19, 2025)

**Status**: ✅ RELEASED

**What's Done**:
1. ✅ **escape.go** - Parser for `go build -gcflags="-m"` output
2. ✅ **escape_test.go** - Unit tests for escape analysis parser
3. ✅ **escape_integration_test.go** - Integration tests
4. ✅ **toolexec.go** - New `racedetector toolexec` command
5. ✅ **InstrumentOptions** - EscapeInfo support in instrumentor
6. ✅ **30-50% instrumentation reduction** on real codebases
7. ✅ **Tested on zygomys** (Issue #17 - false positives eliminated)

**Key Results**:
- lexer.go: 14 writes/35 reads → 9 writes/18 reads (36%/49% reduction)
- Named return value false positives: eliminated
- Function parameter false positives: eliminated

---

### **v0.8.2 - Pointer Dereference Fix** (December 2025) [RELEASED! ✅]

**Goal**: Fix Issue #27 - pointer dereferences like `*ptr++` not instrumented

**Duration**: Same day (December 19, 2025)

**Status**: ✅ RELEASED

**Root Cause**: Go parser uses `*ast.StarExpr` for pointer dereference in `IncDecStmt`, but we only checked `*ast.UnaryExpr`.

**Changes**:
- ✅ Added `*ast.StarExpr` handling in `Visit()` switch
- ✅ Added `visitStarExpr()` for pointer dereference instrumentation
- ✅ Added `StarExpr` handling in `extractReads()` and `extractAddress()`
- ✅ Test cases for Issue #27 reproduction

**Credit**: Thanks to @thepudds for finding this critical bug!

---

### **v0.8.3 - Struct Field Access Fix** (December 2025) [RELEASED! ✅]

**Goal**: Fix Issue #30 - struct field access like `s.x++` not instrumented

**Duration**: Same day (December 19, 2025)

**Status**: ✅ RELEASED

**Root Cause**: `visitSelectorInModifyContext()` recorded `Node: expr` (SelectorExpr), but `findParentStatement()` returned outermost statement (BlockStmt) instead of IncDecStmt.

**Changes**:
- ✅ Modified `visitSelectorInModifyContext(parentStmt, expr)` to accept parent statement
- ✅ Use parent statement as `Node` for correct instrumentation lookup
- ✅ Added `TestInstrumentFile_SelectorExprIncDec` test
- ✅ Added `TestInstrumentFile_SelectorExprInGoroutine` test

**Before fix**:
```go
func update(s *S) {
    s.x++  // No race detection!
}
```

**After fix**:
```go
func update(s *S) {
    race.RaceRead(uintptr(unsafe.Pointer(&s.x)))
    race.RaceWrite(uintptr(unsafe.Pointer(&s.x)))
    s.x++
}
```

**Known Limitation**: Compiler directives (`//go:noinline`) may be misplaced during instrumentation. See Issue #32.

**Credit**: Thanks to @thepudds for finding this critical bug!

---

### **v0.9.0 - Polish & Stabilization** (January 2026) [PLANNED]

**Goal**: Final polish before v1.0.0 LTS release

**Planned Work**:
1. **Documentation improvements**
   - More examples and tutorials
   - Performance tuning guide
   - Migration guide from ThreadSanitizer

2. **Edge case fixes**
   - Any remaining false positive patterns
   - Improved error messages

3. **Performance profiling**
   - Benchmark against ThreadSanitizer
   - Memory usage optimization

4. **Community feedback integration**
   - Address reported issues
   - Feature requests evaluation

**Target**: January 2026

---

### **v0.2.0 - Performance + Production Hardening** (November 2025) [RELEASED! ✅]

**Goal**: ONE production-ready release with both performance and hardening

**Duration**: 2 days (November 19-20, 2025)

**Status**: ✅ ALL 6 TASKS COMPLETE! Exceeded all targets!

**Strategic Decision**: Consolidated v0.3.0 (hardening) into v0.2.0 (performance) for ONE production-ready release. First impression matters!

**Performance Optimizations (Tasks 1-3)**:

1. **CAS-Based Shadow Memory** ✅ RELEASED
   - Replaced sync.Map with atomic.Pointer array
   - **Result**: 81.4% faster (2.07ns vs 11.12ns) - **2× target!**
   - Zero allocations achieved (0 B/op)
   - 34-56% memory savings vs sync.Map
   - <0.01% collision rate (100× better than target)

2. **BigFoot Static Coalescing** ✅ RELEASED
   - Based on "Effective Race Detection for Event-Driven Programs" (PLDI 2017)
   - Coalesces consecutive memory operations at AST level
   - **Result**: 90% barrier reduction (10 barriers → 1 barrier)
   - Exceeds 40-60% target by 30-50%!

3. **SmartTrack Ownership Tracking** ✅ RELEASED
   - Based on "SmartTrack: Efficient Predictive Race Detection" (PLDI 2020)
   - Tracks exclusive writer to skip happens-before checks
   - **Result**: 10-20% HB check reduction (as expected)
   - Single-writer fast path: 30-50% faster

**Production Hardening (Tasks 4-6)**:

4. **Increase MaxThreads and ClockBits** ✅ RELEASED
   - TIDBits: 8 → 16 (256 → 65,536 goroutines, 256× improvement)
   - ClockBits: 24 → 48 (16M → 281T operations, 16M× improvement)
   - Epoch: uint32 → uint64 (16-bit TID + 48-bit clock)
   - **Impact**: Production-scale goroutines and long-running servers

5. **Overflow Detection** ✅ RELEASED
   - Atomic overflow flags (tidOverflowDetected, clockOverflowDetected)
   - 90% warning thresholds (early detection)
   - Periodic checks every 10K operations
   - Clamping to prevent wraparound
   - **Impact**: No more silent failures

6. **Stack Depot** ✅ RELEASED
   - ThreadSanitizer v2 deduplication approach
   - FNV-1a hash for stack traces
   - VarState: 48 → 64 bytes (+ 2 × uint64 hashes)
   - Complete race reports with BOTH stacks!
   - **Impact**: Full debugging information

**Combined Impact**:
- 99% overhead reduction for consecutive operations
- 74× speedup in common patterns
- 2-5% overhead (competitive with ThreadSanitizer!)
- Production-scale: 65K goroutines, 281T operations
- Complete debugging: both current and previous stacks

**Branch**: `feature/v0.2.0-final` (released)

**Completed**: November 20, 2025 (2 days!)

---

### **v0.6.0 - Go Runtime Integration** (January 2026) [PLANNED]

**Goal**: Replace ThreadSanitizer in Go toolchain

**Duration**: 1-2 months (including testing)

**Note**: Renumbered from v0.4.0 to v0.6.0 after adding v0.5.0 (Assembly GID)

**Planned Work**:
1. **Runtime Integration**
   - Replace `runtime/race/*.syso` with pure Go implementation
   - Hook into `go build -race` flag
   - Maintain API compatibility with existing instrumentation

2. **Compiler Coordination**
   - Work with existing `cmd/compile/internal/walk/race.go`
   - Ensure instrumentation calls match new runtime
   - Test with official Go test suite

3. **Validation**
   - Run official Go race detector tests
   - Benchmark against ThreadSanitizer
   - Cross-platform testing (Linux, macOS, Windows)

**Target**: January 31, 2026

---

### **v1.0.0 - Long-Term Support Release** (Q1 2026)

**Goal**: LTS release with Go community adoption

**Requirements**:
- v0.4.0 stable for 1+ months
- Positive Go community feedback
- Performance competitive with ThreadSanitizer (<20% difference)
- No critical bugs
- API proven in production

**LTS Guarantees**:
- ✅ API stability (no breaking changes in v1.x.x)
- ✅ Long-term support (2+ years)
- ✅ Semantic versioning strictly followed
- ✅ Security updates and bug fixes
- ✅ Performance improvements

**Go Proposal**:
- Submit official proposal to golang/go
- Present at Go community meetups/conferences
- Collaborate with Go team for integration

**Target**: Q2 2026 (after validation period)

---

## 📚 Resources

**Academic Foundation**:
- FastTrack paper: [PLDI 2009](https://users.soe.ucsc.edu/~cormac/papers/pldi09.pdf)
- DJIT+ algorithm (original vector clock approach)
- ThreadSanitizer design papers

**Go Integration**:
- Compiler instrumentation: `go/src/cmd/compile/internal/walk/race.go`
- Runtime API: `go/src/runtime/race.go`
- Official race detector: https://go.dev/blog/race-detector

**Development**:
- CONTRIBUTING.md - How to contribute
- docs/ - User guides (INSTALLATION.md, USAGE_GUIDE.md)
- examples/ - Example programs (mutex_protected, channel_sync, simple_race)

---

## 📞 Support

**Documentation**:
- README.md - Project overview and quick start
- INSTALLATION.md - Installation instructions
- USAGE_GUIDE.md - Usage examples and best practices
- CHANGELOG.md - Release history

**Feedback**:
- GitHub Issues - Bug reports and feature requests
- Discussions - Questions and help
- Proposals - Architecture and design discussions

---

## 🔬 Development Approach

**Pure Go Implementation**:
- No CGO dependencies
- FastTrack algorithm from academic paper
- Go idioms and best practices
- Comprehensive testing (unit + integration)

**Quality Standards**:
- ✅ 70%+ test coverage minimum
- ✅ 90%+ for core detector logic
- ✅ 100% test pass rate
- ✅ Zero linter issues (golangci-lint)
- ✅ Professional error messages
- ✅ Comprehensive documentation

**Community First**:
- Open development process
- Regular updates and communication
- Responsive to feedback and bug reports
- Collaborative with Go team

---

*Version 1.6 (Updated 2025-12-19)*
*Current: v0.8.3 (STABLE - Community Bug Fixes) | Next: v0.9.0 (Polish) | Target: v1.0.0 LTS (Q1 2026)*
