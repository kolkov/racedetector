# Pure-Go Race Detector - Development Roadmap

> **Strategic Advantage**: Proven FastTrack algorithm implementation without CGO dependency!
> **Approach**: Scientific algorithm + Go best practices - eliminates C++ ThreadSanitizer dependency

**Last Updated**: 2025-12-10 | **Current Version**: v0.4.10 (STABLE) | v0.5.0 (IN DEVELOPMENT) | **Strategy**: MVP → Optimization + Hardening → Advanced Optimizations → Assembly GID → Runtime Integration → Go Proposal | **Milestone**: v0.5.0 (Assembly GID) → v0.6.0 (Runtime Integration) → v1.0.0 (Q1 2026)

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
v0.5.0 (Assembly GID) 🚧 IN DEVELOPMENT
         ↓ (~2200× faster goroutine ID extraction!)
v0.6.0 (Go Runtime Integration) → Replace ThreadSanitizer in Go toolchain
         ↓ (1-2 months testing)
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

## 📊 Current Status (v0.4.10 Stable | v0.5.0 In Development)

**Phase**: 🚧 Assembly GID Optimization (v0.5.0 branch: feature/assembly-goid)
**Detector**: Production-grade! Now with ~2200× faster goroutine ID! ⚡
**AST Instrumentation**: Complete! Test command fully working! ✅

**What Works**:
- ✅ `racedetector build` command (drop-in for `go build`)
- ✅ `racedetector run` command (drop-in for `go run`)
- ✅ `racedetector test` command (drop-in for `go test -race`) **NEW in v0.4.0!**
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
- ✅ **Professional errors** (file:line:column with suggestions)
- ✅ **Verbose mode** (`-v` flag shows instrumentation stats)
- 🚧 **Assembly GID** (~2200× faster goroutine ID) **IN DEVELOPMENT!**

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

### **v0.5.0 - Assembly-Optimized Goroutine ID** (December 2025) [IN DEVELOPMENT! 🚧]

**Goal**: ~2200× faster goroutine ID extraction with native assembly

**Duration**: 1 day (December 10, 2025)

**Status**: 🚧 Implementation complete, testing and documentation in progress

**Branch**: `feature/assembly-goid`

**What's Done**:
1. ✅ **goid_amd64.s** - Native assembly for x86-64 (TLS access)
2. ✅ **goid_arm64.s** - Native assembly for ARM64 (g register)
3. ✅ **goid_fast.go** - Go wrapper with offset logic (152 bytes)
4. ✅ **goid_fallback.go** - Fallback for unsupported platforms
5. ✅ **goid_generic.go** - Common utilities (parseGID, getGoroutineIDSlow)
6. ✅ **Benchmarks** - 1.73 ns/op (was 3987 ns/op)
7. ✅ **Tests** - All passing including fast vs slow validation
8. 🚧 **Documentation** - Updating README, CHANGELOG, ROADMAP

**Performance Results**:
```
BenchmarkGetGoroutineID_Fast-12    1.73 ns/op    0 B/op    0 allocs/op
BenchmarkGetGoroutineID_Slow-12    3987 ns/op   64 B/op    1 allocs/op
```

**Strategic Decision**: Zero external dependencies!
- Evaluated outrigdev/goid but chose own implementation
- If Go team accepts detector into runtime, GID is their responsibility
- External dependency would complicate Go proposal acceptance

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

*Version 1.4 (Updated 2025-12-10)*
*Current: v0.4.10 (STABLE) | v0.5.0 (IN DEVELOPMENT - Assembly GID) | Next: v0.6.0 (Go Runtime Integration) | Target: v1.0.0 LTS (Q1 2026)*
