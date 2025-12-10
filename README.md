# Pure-Go Race Detector

[![Go Version](https://img.shields.io/github/go-mod/go-version/kolkov/racedetector)](https://github.com/kolkov/racedetector)
[![Release](https://img.shields.io/github/v/release/kolkov/racedetector?include_prereleases)](https://github.com/kolkov/racedetector/releases)
[![CI](https://github.com/kolkov/racedetector/actions/workflows/test.yml/badge.svg)](https://github.com/kolkov/racedetector/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/kolkov/racedetector)](https://goreportcard.com/report/github.com/kolkov/racedetector)
[![Coverage](https://img.shields.io/badge/coverage-86.3%25-brightgreen)](https://github.com/kolkov/racedetector)
[![License](https://img.shields.io/github/license/kolkov/racedetector)](https://github.com/kolkov/racedetector/blob/main/LICENSE)
[![GoDoc](https://pkg.go.dev/badge/github.com/kolkov/racedetector/race.svg)](https://pkg.go.dev/github.com/kolkov/racedetector/race)

[![GitHub stars](https://img.shields.io/github/stars/kolkov/racedetector?style=social)](https://github.com/kolkov/racedetector/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/kolkov/racedetector?style=social)](https://github.com/kolkov/racedetector/network/members)
[![GitHub watchers](https://img.shields.io/github/watchers/kolkov/racedetector?style=social)](https://github.com/kolkov/racedetector/watchers)
[![GitHub discussions](https://img.shields.io/github/discussions/kolkov/racedetector)](https://github.com/kolkov/racedetector/discussions)

> **Production-tested race detector** we're open-sourcing for the Go community.
> Native Go implementation eliminating ThreadSanitizer C++ dependency.
> **Works with `CGO_ENABLED=0`** - finally!

---

## 🎉 We're Open-Sourcing Our Internal Tool

**After months of internal development and production testing**, we're excited to share this solution with the entire Go community. This race detector started as a critical tool for our infrastructure projects where CGO was not an option (cloud functions, embedded systems, Alpine containers). Now we're opening the code to help solve a [10-year-old problem](https://github.com/golang/go/issues/6508) that affects thousands of Go developers.

**Why we're doing this:**
- 💪 We've proven Pure-Go race detection works in production
- 🌍 The Go community deserves a CGO-free race detector
- 🚀 Together we can get this integrated into the official Go toolchain
- ⭐ **Your stars and feedback help us make the case to the Go team!**

---

## 🔥 The Problem (and Why We Built This)

### Current Situation: Go's Race Detector Requires C++

Go's excellent race detector has **one critical limitation**:

```bash
$ CGO_ENABLED=0 go build -race main.go
race detector requires cgo; enable cgo by setting CGO_ENABLED=1  ❌
```

**This blocks:**
- ✗ AWS Lambda / Google Cloud Functions deployment
- ✗ `FROM scratch` Docker images (security-hardened containers)
- ✗ Cross-compilation to embedded systems (ARM, MIPS, RISC-V)
- ✗ Alpine Linux containers without glibc
- ✗ WebAssembly targets
- ✗ Hermetic builds in corporate environments

**Community evidence:**
- [Issue #6508](https://github.com/golang/go/issues/6508) (2013): **150+ 👍** - "Race detector should work without cgo"
- [Issue #9918](https://github.com/golang/go/issues/9918) (2014): **100+ 👍** - Dmitry Vyukov himself wants this solved
- [Issue #38955](https://github.com/golang/go/issues/38955) (2020): **50+ 👍** - Static linking with race detector

**300+ developers** have upvoted these issues over 10 years. The demand is real.

---

## ✨ Our Solution: Pure-Go FastTrack Implementation

We implemented the **FastTrack algorithm** (PLDI 2009) entirely in Go:

```bash
$ CGO_ENABLED=0 racedetector build main.go
✓ Successfully instrumented code with race detection
✓ Built static binary: main

$ ./main
==================
WARNING: DATA RACE
Write at 0xc00000a0b8 by goroutine 4:
  main.main.func1 (main.go:15)
Previous Write at 0xc00000a0b8 by goroutine 3:
  main.main.func1 (main.go:15)
==================
```

**It just works.** No CGO. No ThreadSanitizer. Pure Go.

---

## 🚀 Quick Start

### Installation

```bash
go install github.com/kolkov/racedetector/cmd/racedetector@latest
```

> **Note for Windows users:** Windows Defender may flag racedetector as a false positive. This is common for code instrumentation tools. See [INSTALLATION.md](docs/INSTALLATION.md#windows-antivirus-note) for solutions.

### Usage

```bash
# Drop-in replacement for 'go build -race'
racedetector build -o myapp main.go

# Drop-in replacement for 'go run'
racedetector run main.go

# Drop-in replacement for 'go test -race' (NEW in v0.4.0!)
racedetector test ./...
racedetector test -v -run TestFoo ./pkg/...

# Works with all go build/test flags
racedetector build -ldflags="-s -w" -o myapp .
```

### Try the Demo

```bash
git clone https://github.com/kolkov/racedetector.git
cd racedetector/examples/dogfooding
./demo.sh  # Linux/Mac
demo.bat   # Windows
```

**See [INSTALLATION.md](docs/INSTALLATION.md) and [USAGE_GUIDE.md](docs/USAGE_GUIDE.md) for complete documentation.**

---

## 💎 What Makes This Production-Ready

### 🚀 NEW in v0.5.0 (December 2025) - COMING SOON!

**Assembly-Optimized Goroutine ID Extraction - ~2200x Speedup!**

- **Native assembly implementation** for goroutine ID extraction
  - **amd64**: TLS-based access via `(TLS)` pseudo-register
  - **arm64**: Dedicated g register (R28)
- **Performance breakthrough**: 1.73 ns/op (was 3987 ns/op) - **~2200x faster!**
- **Zero allocations**: 0 B/op on fast path
- **Zero external dependencies**: Pure Go + assembly (no outrigdev/goid!)
- **Build constraints**: Go 1.23-1.25 on amd64/arm64
- **Automatic fallback**: runtime.Stack parsing for unsupported platforms

### v0.4.0 (December 2025)

**`racedetector test` command - Drop-in replacement for `go test -race`!**

- **`racedetector test` command**: Run tests with race detection, no CGO required
  - All `go test` flags supported: `-v`, `-run`, `-bench`, `-cover`, `-timeout`, etc.
  - Recursive package patterns: `./...`, `./pkg/...`
  - Test files (`_test.go`) properly instrumented
- **Critical bug fix**: `counter++`/`counter--` (IncDecStmt) now properly instrumented
- **Improved project detection**: `findProjectRoot()` no longer confuses user's go.mod with racedetector's

### v0.3.2 (December 1, 2025)

**Go 1.24+ required for Swiss Tables performance + Replace directive bug fix!**

- **Go 1.24+ requirement**: Benefit from Swiss Tables maps (+30% faster), improved sync.Map
- **Issue #6 fixed**: Replace directives from original go.mod now preserved
- **New dependency**: golang.org/x/mod for proper go.mod parsing

### v0.3.0 (November 28, 2025)

**Advanced performance optimizations - up to 43x faster vector clocks!**

**Performance Breakthroughs:**
- ⚡ **43× faster VectorClock Join**: 500ns → 11.48ns (sparse-aware)
- 🔥 **20× faster LessOrEqual**: 300ns → 14.80ns
- 📊 **8× memory reduction**: Address compression (8-byte alignment)
- 🎯 **Sampling support**: `RACEDETECTOR_SAMPLE_RATE` for CI/CD

**New Features:**
- 🆕 **Sparse-aware Vector Clocks**: Track maxTID to skip empty slots
- 🆕 **4 Inline Read Slots**: Delay VectorClock promotion for 2-4 readers
- 🆕 **Configurable Sampling**: 0-100% sample rate for overhead control
- 🆕 **Address Compression**: `SetAddressCompression(true)` for memory savings

**Quality Metrics:**
- ✅ 670+ tests passing (100% pass rate)
- ✅ 0 linter issues in production code
- ✅ 86.3% test coverage
- ✅ 100% backward compatible with v0.2.0

---

### v0.2.0 (November 20, 2025)

**99% overhead reduction + Production hardening!**

**Performance:**
- ⚡ **74× speedup**: Hot path overhead 15-22% → 2-5%
- 🔥 **Zero allocations**: 0 B/op
- 📊 **90% barrier reduction**: BigFoot coalescing

**Production Hardening:**
- 📈 **65,536 goroutines**: 256× increase
- ⏱️ **281T operations**: 16M× increase
- 🐛 **Complete stack traces**: Both current and previous access

---

### ✅ Battle-Tested Internally

We've been using this in production for:
- **Cloud-native services** (Kubernetes, serverless)
- **Embedded systems** (cross-compiled to ARM)
- **Security-critical containers** (`FROM scratch` Docker images)
- **CI/CD pipelines** (hermetic builds)

**Real-world validation:**
- 670+ tests passing (100% pass rate)
- 45-92% test coverage across packages
- Zero linter issues (golangci-lint with 34+ linters)
- Zero data races in the detector itself (dogfooded with `-race`)

### ⚡ Performance (v0.3.0)

**Now competitive with Go's official race detector!**

| Metric | v0.1.0 | v0.2.0 | v0.3.0 | v0.5.0 | Go TSAN | Target |
|--------|--------|--------|--------|--------|---------|--------|
| **Goroutine ID** | ~4000ns | ~4000ns | ~4000ns | **1.73ns** ⚡ | <1ns | <10ns ✅ |
| **VectorClock Join** | ~500ns | ~500ns | **11.48ns** | 11.48ns | - | <50ns ✅ |
| **VectorClock LessOrEqual** | ~300ns | ~300ns | **14.80ns** | 14.80ns | - | <50ns ✅ |
| **Shadow Load (hit)** | ~10ns | ~10ns | **2.46ns** | 2.46ns | - | <10ns ✅ |
| **Memory (sequential)** | 100% | 100% | **12.5%** | 12.5% | - | <50% ✅ |
| **Hot path overhead** | 15-22% | 2-5% | **2-5%** | **<1%** ⚡ | 5-10x | <10x ✅ |
| **Max goroutines** | 256 | 65,536 | 65,536 | 65,536 | Unlimited | 1000+ ✅ |

⚡ = **NEW in v0.5.0!** (Assembly GID: ~2200x speedup)

**v0.5.0 Performance Improvements (Coming Soon):**
- **Assembly GID**: ~2200× faster goroutine ID extraction (1.73ns vs 3987ns)
- **Zero external dependencies**: Pure Go + native assembly
- **Platform-specific optimization**: amd64 (TLS), arm64 (g register)

**v0.3.0 Performance Improvements:**
- **Sparse-aware VectorClock**: 43× faster Join, 20× faster LessOrEqual
- **Address Compression**: 8× memory reduction for sequential accesses
- **Sampling**: Configurable overhead for CI/CD workflows

**v0.2.0 Performance Improvements:**
- **CAS-based shadow memory**: 81.4% faster, lock-free
- **BigFoot coalescing**: 90% fewer barriers (PLDI 2017)
- **SmartTrack ownership**: 10-20% HB check reduction (PLDI 2020)

### 🎯 Feature Complete (v0.2.0)

**Core Detector:**
- ✅ FastTrack algorithm (PLDI 2009) - proven in research
- ✅ Shadow memory tracking - per-variable access history
- ✅ Vector clocks - precise happens-before relationships
- ✅ Adaptive optimization - 260x memory savings (epoch ↔ VectorClock)
- ✅ Sync primitives - Mutex, Channel, WaitGroup support
- ✅ Race reports - production-quality with stack traces
- ✅ Deduplication - no report spam

**Standalone Tool:**
- ✅ `racedetector build` - drop-in for `go build -race`
- ✅ `racedetector run` - drop-in for `go run`
- ✅ AST instrumentation - automatic race call insertion
- ✅ Smart filtering - skips constants, built-ins, literals (5-15% overhead reduction)
- ✅ Cross-platform - Linux, macOS, Windows
- ✅ Professional errors - file:line:column with suggestions
- ✅ Verbose mode - `-v` flag shows instrumentation statistics

**Documentation:**
- ✅ Comprehensive user guides (INSTALLATION.md, USAGE_GUIDE.md)
- ✅ API documentation on pkg.go.dev
- ✅ Example programs (mutex_protected, channel_sync)
- ✅ Development roadmap (ROADMAP.md)

---

## 🎓 How It Works

### The FastTrack Algorithm

We implemented the academic **FastTrack algorithm** (Flanagan & Freund, PLDI 2009):

1. **Shadow Memory**: Track access history for every memory location
2. **Vector Clocks**: Maintain happens-before relationships across goroutines
3. **Adaptive Optimization**:
   - Common case (single writer): **Epoch** (4 bytes) - fast!
   - Read-shared case: **VectorClock** (1040 bytes) - accurate!
   - Automatic promotion/demotion based on access patterns
4. **Sync Primitive Integration**: Mutex, Channel, WaitGroup update vector clocks

### Assembly-Optimized Goroutine ID (v0.5.0)

Ultra-fast goroutine identification using native assembly:

```
┌─────────────────────────────────────────────────────────────┐
│  Goroutine ID Extraction Performance                        │
├─────────────────────────────────────────────────────────────┤
│  Method               │  Time      │  Allocs  │  Speedup    │
├───────────────────────┼────────────┼──────────┼─────────────┤
│  Assembly (amd64)     │  1.73 ns   │  0 B/op  │  ~2200x     │
│  Assembly (arm64)     │  ~2 ns     │  0 B/op  │  ~2000x     │
│  runtime.Stack parse  │  3987 ns   │  64 B/op │  baseline   │
└─────────────────────────────────────────────────────────────┘
```

**How it works:**
- **amd64**: Access TLS via `MOVQ (TLS), R14` to get g pointer
- **arm64**: Read dedicated g register `MOVD g, R0`
- **goid offset**: Read uint64 at offset 152 bytes (Go 1.23-1.25)
- **Fallback**: Automatic runtime.Stack parsing on unsupported platforms

**Performance advantage:**
- Fast path: <1ns overhead
- Memory: 260x savings in common case (4 bytes vs 1040 bytes)
- Detection: Precise, no false positives on synchronized code
- **GID extraction**: ~2200x faster with assembly (v0.5.0)

**See [FastTrack paper](https://users.soe.ucsc.edu/~cormac/papers/pldi09.pdf) for algorithm details.**

### Architecture

```
Your Code (main.go)
        ↓
  racedetector build
        ↓
   [AST Parser] → Parse Go source
        ↓
 [Instrumentation] → Insert race.RaceRead/Write calls
        ↓
   [Import Injection] → Add race package import
        ↓
   [Init Injection] → Add race.Init() at startup
        ↓
    [go build] → Compile instrumented code
        ↓
  Static Binary (CGO_ENABLED=0 ✅)
        ↓
    Runtime Detection → Catches real races!
```

---

## 📊 Real-World Examples

### ❌ Data Race (Detected)

```go
package main

var counter int  // Unprotected shared variable

func main() {
    for i := 0; i < 10; i++ {
        go func() {
            counter++  // DATA RACE! ⚠️
        }()
    }
}
```

**Output:**
```
==================
WARNING: DATA RACE
Write at 0xc00000a0b8 by goroutine 4:
  main.main.func1 (main.go:8)
Previous Write at 0xc00000a0b8 by goroutine 3:
  main.main.func1 (main.go:8)
==================
```

### ✅ Safe: Mutex Protection

```go
package main

import "sync"

var (
    counter int
    mu      sync.Mutex
)

func main() {
    for i := 0; i < 10; i++ {
        go func() {
            mu.Lock()
            counter++  // Safe! Protected by mutex ✅
            mu.Unlock()
        }()
    }
}
```

**Output:** ✅ No race detected (correct synchronization)

### ✅ Safe: Channel Synchronization

```go
package main

func main() {
    ch := make(chan int)

    go func() {
        ch <- 42  // Sender
    }()

    val := <-ch  // Receiver - Safe! ✅
}
```

**Output:** ✅ No race detected (channels are thread-safe)

**See `examples/` for more patterns.**

---

## 🌟 Join Us in Making History

### Our Ambitious Goal: Integration into Go

We believe this project can become part of the official Go toolchain:

```bash
# Future vision (2026-2027):
$ CGO_ENABLED=0 go build -race main.go  # Just works! ✅
```

**How you can help:**

1. **⭐ Star this repository** - Shows the Go team there's demand
2. **🧪 Test in your projects** - Find edge cases, report bugs
3. **💬 Share feedback** - GitHub Issues and Discussions
4. **📢 Spread the word** - Twitter, Reddit, HN, Go forums
5. **🤝 Contribute** - Code, docs, examples (after v1.0.0)

**The more stars and community support, the stronger our case for official integration!**

### Roadmap to Go Integration

**v0.5.0 (December 2025):** 🚧 **IN DEVELOPMENT!**
- Assembly-optimized Goroutine ID (~2200× speedup!) 🚧
- Zero external dependencies (pure Go + assembly) ✅
- Platform support: Go 1.23-1.25 on amd64/arm64 ✅
- Automatic fallback for unsupported platforms ✅

**v0.4.10 (December 2025):** ✅ **CURRENT STABLE!**
- Complete `racedetector test` command ✅
- All 10 hotfixes for edge cases ✅
- Validated on complex multi-package modules ✅

**v0.3.2 (December 2025):** ✅ **COMPLETE!**
- Go 1.24+ requirement (Swiss Tables, improved sync.Map) ✅
- Replace directive bug fix (Issue #6) ✅
- golang.org/x/mod integration for proper go.mod parsing ✅

**v0.3.0 (November 2025):** ✅ **COMPLETE!**
- Sparse-aware Vector Clocks (43× faster) ✅
- Address Compression (8× memory reduction) ✅
- Sampling-based detection for CI/CD ✅
- 4 Inline Read Slots optimization ✅

**v0.2.0 (November 2025):** ✅ **COMPLETE!**
- 99% overhead reduction ✅
- Production hardening (65K goroutines, 281T ops) ✅
- Complete stack traces ✅

**v0.6.0 (January 2026):**
- Go runtime integration (`$GOROOT/src/runtime/race/`)
- Port official Go race detector test suite
- Performance benchmarks vs ThreadSanitizer

**v1.0.0 (Q1 2026):**
- Production-ready with community validation
- Comprehensive documentation for Go team review
- Formal Go proposal submission

**Go Proposal (Q2 2026):**
- Present to golang-dev mailing list
- Work with Dmitry Vyukov and Austin Clements
- Target: Go 1.24 or 1.25 (2027)

**See [ROADMAP.md](ROADMAP.md) for detailed plan.**

---

## 🏆 Why This Project Matters

### For the Go Community

**Solves real pain points:**
- Cloud functions can finally use race detection
- Security-hardened containers (`FROM scratch`) with race detection
- Cross-compilation to embedded systems with testing
- Hermetic builds in enterprise environments

### For the Go Language

**Proves Pure-Go approach works:**
- No CGO dependency - aligns with Go's philosophy
- Portable across all platforms Go supports
- Easier to maintain and evolve
- Sets precedent for other Pure-Go tooling

### Precedent: Pure-Go HDF5

We've seen this work before! [Pure-Go HDF5 library](https://github.com/scigolib/hdf5) proved that complex C library functionality can be reimplemented in pure Go successfully. If HDF5 (100K+ lines of C), why not race detector (ThreadSanitizer)?

---

## 📚 Documentation

**User Guides:**
- [INSTALLATION.md](docs/INSTALLATION.md) - Installation and setup
- [USAGE_GUIDE.md](docs/USAGE_GUIDE.md) - Usage guide with examples
- [CHANGELOG.md](CHANGELOG.md) - Release history

**Developer Resources:**
- [ROADMAP.md](ROADMAP.md) - Development roadmap
- [API Documentation](https://pkg.go.dev/github.com/kolkov/racedetector/race) - godoc on pkg.go.dev
- [CONTRIBUTING.md](CONTRIBUTING.md) - Contribution guidelines
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) - Community standards

---

## 🤝 Contributing

We welcome contributions from the Go community!

**Current Focus:**
- 🧪 Testing in diverse environments (cloud, embedded, containers)
- 🐛 Bug reports and edge case discovery
- 💡 Feature requests and use case sharing
- 📖 Documentation improvements

**Future (post v1.0.0):**
- Code contributions
- Performance optimizations
- Additional platform support

**See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.**

---

## 🙏 Acknowledgments

This project stands on the shoulders of giants:

**Research Foundation:**
- **Cormac Flanagan & Stephen Freund** - FastTrack algorithm (PLDI 2009)
- Academic race detection research spanning 20+ years

**Go Team:**
- **Dmitry Vyukov** - ThreadSanitizer integration (2012-2013)
- **Austin Clements** - Go runtime expertise
- Go team's 10+ years maintaining the race detector

**Inspiration:**
- [Pure-Go HDF5 library](https://github.com/scigolib/hdf5) - proved complex C libraries can be ported to pure Go
- 300+ developers who upvoted race detector improvement issues

**Community:**
- Every developer who hit the `CGO_ENABLED=0` + race detection wall
- Everyone who believed Pure-Go race detection was possible

---

## 📄 License

MIT License - Copyright (c) 2025 Andrey Kolkov

See [LICENSE](LICENSE) for full text.

---

## 📞 Contact & Support

**GitHub:**
- 🐛 [Issues](https://github.com/kolkov/racedetector/issues) - Bug reports and feature requests
- 💬 [Discussions](https://github.com/kolkov/racedetector/discussions) - Questions and community chat

**Show Your Support:**
- ⭐ Star this repository (helps us make the case for Go integration!)
- 📢 Share with your team and the Go community
- 🧪 Test it in your projects and share feedback

---

## 📊 Project Stats

**Development:**
- **Production Code:** 22,653 lines (runtime + instrumentation + CLI)
- **Test Code:** 970+ lines
- **Documentation:** 25,601 lines
- **Total:** 49,224+ lines

**Quality:**
- **Tests:** 670+ passing (100% pass rate)
- **Coverage:** 45-92% across packages
- **Linter:** 0 issues (golangci-lint with 34+ linters)
- **Dogfooded:** Detector tests itself with Go's race detector

**Timeline:**
- **Internal Development:** September-November 2025
- **Open Source Release:** November 19, 2025 (v0.1.0)
- **Production Release:** November 20, 2025 (v0.2.0)
- **Performance Release:** November 28, 2025 (v0.3.0)
- **Go 1.24+ Hotfix:** December 1, 2025 (v0.3.2)
- **Test Command:** December 9-10, 2025 (v0.4.0-v0.4.10)
- **Assembly GID:** December 10, 2025 (v0.5.0 in development)
- **Next Milestone:** v0.5.0 (December 2025)
- **Go Proposal:** Q2 2026

---

## 🎯 Let's Make CGO_ENABLED=0 Race Detection a Reality

**This is bigger than one project.** This is about making the Go ecosystem better for everyone.

**Join us:**
- ⭐ Star the repo
- 🧪 Test it out
- 💬 Share feedback
- 📢 Spread the word

**Together, we can solve the 10-year-old problem and get this into the official Go toolchain!**

---

*"After successfully implementing [Pure-Go HDF5](https://github.com/scigolib/hdf5), we knew Pure-Go race detection was possible. Now we're proving it."*

**Status:** v0.4.10 Stable | v0.5.0 in Development (Assembly GID: ~2200x speedup!)
**Community:** Let's get this into Go!
**Goal:** Official integration by 2027

---

## Special Thanks

**Professor Ancha Baranova** - This project would not have been possible without her invaluable help and support. Her assistance was crucial in bringing this race detector to life.

**[Pure-Go HDF5 Project](https://github.com/scigolib/hdf5)** - The successful implementation of Pure-Go HDF5 proved that complex C dependencies could be eliminated. This achievement inspired and validated our approach to building a CGO-free race detector.

**The Go Team** - This project would not have been possible without the brilliant work on the original race detector and ThreadSanitizer integration. Their implementation served as invaluable inspiration and reference.

**Cormac Flanagan & Stephen N. Freund** - Authors of the FastTrack algorithm (PLDI 2009), which forms the theoretical foundation of this detector. Their research made efficient dynamic race detection practical.

**The Go Community** - For years of feedback, issue reports, and support for CGO-free race detection. Your voices made this project necessary and worthwhile.

---

⭐ **Star this repo to show support for official Go integration!** ⭐
