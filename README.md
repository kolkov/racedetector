# Pure-Go Race Detector

[![Go Version](https://img.shields.io/github/go-mod/go-version/kolkov/racedetector)](https://github.com/kolkov/racedetector)
[![Release](https://img.shields.io/github/v/release/kolkov/racedetector)](https://github.com/kolkov/racedetector/releases)
[![CI](https://github.com/kolkov/racedetector/actions/workflows/test.yml/badge.svg)](https://github.com/kolkov/racedetector/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/kolkov/racedetector)](https://goreportcard.com/report/github.com/kolkov/racedetector)
[![Coverage](https://img.shields.io/badge/coverage-86.3%25-brightgreen)](https://github.com/kolkov/racedetector)
[![License](https://img.shields.io/github/license/kolkov/racedetector)](https://github.com/kolkov/racedetector/blob/main/LICENSE)
[![GoDoc](https://pkg.go.dev/badge/github.com/kolkov/racedetector/race.svg)](https://pkg.go.dev/github.com/kolkov/racedetector/race)

> **Pure Go race detector that works with `CGO_ENABLED=0`.**
> Drop-in replacement for `go build -race` and `go test -race`.

---

## The Problem

Go's race detector requires CGO and ThreadSanitizer:

```bash
$ CGO_ENABLED=0 go build -race main.go
race detector requires cgo; enable cgo by setting CGO_ENABLED=1
```

This blocks race detection for:
- AWS Lambda / Google Cloud Functions
- `FROM scratch` Docker images
- Cross-compilation to embedded systems
- Alpine Linux containers
- WebAssembly targets
- Hermetic builds

**This project solves it.** Pure Go implementation, no CGO required.

---

## Installation

```bash
go install github.com/kolkov/racedetector/cmd/racedetector@latest
```

**Requirements:** Go 1.24+

---

## Usage

```bash
# Build with race detection (replaces: go build -race)
racedetector build -o myapp main.go

# Run with race detection (replaces: go run -race)
racedetector run main.go

# Test with race detection (replaces: go test -race)
racedetector test ./...
racedetector test -v -run TestFoo ./pkg/...

# Toolexec mode (for complex build systems)
go build -toolexec="racedetector toolexec" ./...
```

All standard `go build`, `go run`, and `go test` flags are supported.

**v0.8.5+:** Full support for projects with `internal/` packages and `//go:embed` via Go overlay architecture.
**v0.8.0+:** Escape analysis integration reduces false positives by 30-50%.

---

## Example

**Buggy code:**
```go
package main

import "sync"

var counter int

func main() {
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            counter++ // DATA RACE
        }()
    }
    wg.Wait()
}
```

**Detection:**
```bash
$ racedetector build -o app main.go && ./app
==================
WARNING: DATA RACE
Write at 0xc00000a0b8 by goroutine 7:
  main.main.func1 (main.go:13)
Previous Write at 0xc00000a0b8 by goroutine 6:
  main.main.func1 (main.go:13)
==================
```

---

## How It Works

Implements the **FastTrack algorithm** (Flanagan & Freund, PLDI 2009):

1. **AST Instrumentation** - Parses Go source, inserts race detection calls
2. **Shadow Memory** - Tracks access history for every memory location
3. **Vector Clocks** - Maintains happens-before relationships across goroutines
4. **Sync Primitive Tracking** - Respects Mutex, Channel, WaitGroup synchronization

**Architecture:**
```
Source Code → AST Parser → Instrumentation → go build → Static Binary
                                                            ↓
                                                    Runtime Detection
```

---

## Performance

| Metric | racedetector | Go TSAN |
|--------|--------------|---------|
| Goroutine ID extraction | 1.73 ns | <1 ns |
| VectorClock operations | 11-15 ns | - |
| Shadow memory access | 2-4 ns | - |
| Hot path overhead | 2-5% | 5-10x |
| Max goroutines | 65,536 | Unlimited |

Assembly-optimized goroutine ID extraction on amd64/arm64. Automatic fallback for other platforms.

---

## Test Coverage

Ported **359 test scenarios** from Go's official race detector test suite:
- Write-write and read-write races
- Mutex, Channel, WaitGroup synchronization
- Struct fields, slices, maps, arrays
- Closures, defer, panic/recover
- Atomic operations, type assertions

**100% pass rate** with `GOMAXPROCS=1` (same configuration as Go's official tests).

---

## Limitations

- Performance overhead higher than ThreadSanitizer for some workloads
- Struct field access via dot notation has limited coverage
- Assembly optimization only on amd64/arm64 (fallback available)
- `build` command does not yet use overlay architecture (copies files to temp dir)

### Atomic Operations

Like Go's built-in race detector (ThreadSanitizer), this tool is a **dynamic race detector** based on happens-before analysis. It cannot detect races that depend on:

- **Compiler/CPU memory reordering** - The detector observes actual execution order, not all possible reorderings
- **Weak memory model semantics** - Atomic operations provide acquire/release barriers, but the detector uses full synchronization for tracking

This is a fundamental limitation of all dynamic race detectors, not specific to this implementation. For detecting memory model violations, consider static analysis tools or formal verification.

For more details, see [Go Race Detector documentation](https://go.dev/doc/articles/race_detector).

---

## Documentation

- [Installation Guide](docs/INSTALLATION.md)
- [Usage Guide](docs/USAGE_GUIDE.md)
- [Changelog](CHANGELOG.md)
- [API Reference](https://pkg.go.dev/github.com/kolkov/racedetector/race)

---

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Current priorities:**
- Testing in diverse environments
- Bug reports and edge cases
- Documentation improvements

---

## Maintainers

- **[@kolkov](https://github.com/kolkov)** - Creator & Lead Maintainer

---

## Acknowledgments

**Research:**
- **Cormac Flanagan & Stephen Freund** - FastTrack algorithm (PLDI 2009)
- **Dmitry Vyukov** - ThreadSanitizer and Go race detector integration
- **Go Team** - Original race detector implementation

**Community Contributors:**
- **[@thepudds](https://github.com/thepudds)** - Critical bug report: pointer dereference instrumentation ([#27](https://github.com/kolkov/racedetector/issues/27))
- **[@glycerine](https://github.com/glycerine)** - Extensive real-world testing on [zygomys](https://github.com/glycerine/zygomys) ([#15](https://github.com/kolkov/racedetector/issues/15), [#16](https://github.com/kolkov/racedetector/issues/16), [#17](https://github.com/kolkov/racedetector/issues/17))

---

## License

MIT License - see [LICENSE](LICENSE)

---

## Support

- [Issues](https://github.com/kolkov/racedetector/issues) - Bug reports
- [Discussions](https://github.com/kolkov/racedetector/discussions) - Questions

**Goal:** Integration into official Go toolchain. Your feedback helps make the case.
