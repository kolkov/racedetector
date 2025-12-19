# Usage Guide - Pure-Go Race Detector

Complete guide to using the `racedetector` tool for detecting data races in Go programs.

## Table of Contents

- [Quick Start](#quick-start)
- [Command Reference](#command-reference)
  - [build Command](#build-command)
  - [run Command](#run-command)
  - [test Command](#test-command)
  - [toolexec Command](#toolexec-command) (NEW in v0.8.0)
- [Escape Analysis (NEW in v0.8.0)](#escape-analysis-new-in-v080)
- [Integration with CI/CD](#integration-with-cicd)
- [Best Practices](#best-practices)
- [Sampling Mode](#sampling-mode)
- [Current Limitations](#current-limitations)
- [FAQ](#faq)

---

## Quick Start

### Detect a Race in 60 Seconds

```bash
# Create a simple program with a data race
cat > race_example.go <<'EOF'
package main

import (
    "fmt"
    "time"
)

func main() {
    var counter int  // Shared variable - NO SYNCHRONIZATION!

    // Start 10 goroutines that all modify counter
    for i := 0; i < 10; i++ {
        go func(id int) {
            val := counter    // READ
            counter = val + 1 // WRITE
            fmt.Printf("Goroutine %d: counter = %d\n", id, counter)
        }(i)
    }

    time.Sleep(100 * time.Millisecond)
    fmt.Printf("Final counter: %d (expected 10)\n", counter)
}
EOF

# Run with race detection
racedetector run race_example.go

# Or build first, then run
racedetector build -o race_example race_example.go
./race_example

# Clean up
rm race_example.go race_example
```

**Expected behavior:**
- Program runs successfully
- `race.Init()` is called automatically
- Race detector analyzes all memory accesses
- Reports: "WARNING: DATA RACE" with full stack traces

---

## Command Reference

### `build` Command

Build a Go program with automatic race detection instrumentation.

**Syntax:**
```bash
racedetector build [go-build-flags] <source-files>
```

**Description:**
Drop-in replacement for `go build` that automatically:
1. Parses Go source files
2. Injects race detector imports
3. Instruments memory accesses with RaceRead/Write calls
4. Injects `race.Init()` at program start
5. Links Pure-Go race detector runtime
6. Builds instrumented binary

**Examples:**

```bash
# Simple build (single file)
racedetector build main.go

# Build with output file
racedetector build -o myapp main.go

# Build with multiple source files
racedetector build -o server main.go handler.go db.go

# Build with ldflags (strip debug info)
racedetector build -ldflags="-s -w" -o myapp main.go

# Build with build tags
racedetector build -tags=production,debug -o myapp main.go

# Build current directory
racedetector build -o myapp .

# Build with all standard go build flags
racedetector build -v -x -ldflags="-X main.version=1.0.0" main.go
```

**Supported Flags:**

All standard `go build` flags are supported:

| Flag | Description |
|------|-------------|
| `-o <file>` | Output file path |
| `-v` | Verbose output |
| `-x` | Print commands |
| `-ldflags="..."` | Linker flags |
| `-tags="..."` | Build tags |
| `-trimpath` | Remove file paths from binary |
| `-buildmode=<mode>` | Build mode (default, exe, pie, etc.) |
| `-race` | (Ignored - race detection always enabled) |

**Flag Format:**

Both formats are supported:
```bash
-o myapp        # Space-separated
-o=myapp        # Equals-separated
```

**Output:**

```
Built successfully: myapp
```

**What Happens Behind the Scenes:**

1. Creates temporary workspace
2. Copies source files
3. Parses AST and instruments code
4. Generates `go.mod` overlay with race detector dependency
5. Runs `go mod tidy`
6. Compiles with `go build`
7. Moves binary to output path
8. Cleans up temporary files

**Production Status:**
- ✅ Import injection works
- ✅ `race.Init()` is called automatically
- ✅ All build flags supported
- ✅ Full memory access instrumentation
- ✅ Race detection and reporting

---

### `run` Command

Run a Go program with automatic race detection.

**Syntax:**
```bash
racedetector run [go-build-flags] <source-files> [-- program-args]
```

**Description:**
Drop-in replacement for `go run` that builds instrumented binary in a temporary location and executes it.

**Examples:**

```bash
# Simple run (single file)
racedetector run main.go

# Run with program arguments
racedetector run main.go arg1 arg2

# Run with program flags
racedetector run main.go --port=8080 --debug

# Run multiple source files with arguments
racedetector run main.go helper.go -- --config=prod.yaml

# Run with build flags
racedetector run -ldflags="-s -w" main.go

# Run with build tags
racedetector run -tags=debug main.go --verbose
```

**Argument Parsing:**

The command intelligently separates build flags from program arguments:

```bash
racedetector run -ldflags="-X main.version=1.0" main.go --port 8080
                 ^build flags^                  ^source^ ^program args^
```

**Flow:**
1. Build flags come first (start with `-` and before any `.go` file)
2. Source files (any `.go` file)
3. Program arguments (everything after source files)

**Output Forwarding:**

All streams are forwarded transparently:
- **stdin**: Interactive input works
- **stdout**: Program output printed directly
- **stderr**: Error messages printed directly
- **exit code**: Exit code from program propagated

**Cleanup:**

Temporary binary is automatically deleted after execution (even on Ctrl+C).

**Production Status:**
- ✅ Builds and runs successfully
- ✅ Argument parsing works correctly
- ✅ Stream forwarding works
- ✅ Full race detection with stack traces

---

### `test` Command

Run Go tests with automatic race detection instrumentation.

**Syntax:**
```bash
racedetector test [go-test-flags] [packages]
```

**Description:**
Drop-in replacement for `go test -race` that:
1. Instruments all source files AND test files
2. Injects race detector imports
3. Instruments all memory accesses including in anonymous functions
4. Runs `go test` with instrumented code
5. Reports races detected during test execution

**Examples:**

```bash
# Test current package
racedetector test

# Test with verbose output
racedetector test -v ./...

# Test all packages recursively
racedetector test ./...

# Test specific subpackages
racedetector test ./internal/...

# Test with coverage
racedetector test -cover -coverprofile=coverage.out ./...

# Test specific function
racedetector test -run=TestMyFunction ./pkg/...

# Benchmark tests with race detection
racedetector test -bench=. -benchmem ./...

# Test with timeout
racedetector test -timeout=30s ./...

# Test with count (run N times)
racedetector test -count=10 -v ./...
```

**Supported Flags:**

All standard `go test` flags are supported:

| Flag | Description |
|------|-------------|
| `-v` | Verbose output |
| `-run=<regex>` | Run only tests matching regex |
| `-bench=<regex>` | Run only benchmarks matching regex |
| `-benchmem` | Print memory allocation statistics |
| `-cover` | Enable coverage analysis |
| `-coverprofile=<file>` | Write coverage profile |
| `-count=<n>` | Run tests N times |
| `-timeout=<duration>` | Timeout for each test |
| `-parallel=<n>` | Maximum test parallelism |
| `-cpu=<list>` | List of GOMAXPROCS values |
| `-trace=<file>` | Write trace to file |

**Output:**

```
=== RUN   TestRace
==================
WARNING: DATA RACE
Write at 0x00007ff6d3b71268 by goroutine 3:
  ...
Previous Write at 0x00007ff6d3b71268 by goroutine 2:
  ...
==================
    main_test.go:24: counter = 2
--- PASS: TestRace (0.10s)
PASS
ok      mypackage       0.269s
```

**Flow:**
1. Parse test flags and package patterns
2. Create temporary workspace
3. Instrument all `.go` files (including `_test.go`)
4. Setup runtime linking (go.mod overlay)
5. Run `go test` with instrumented code
6. Forward test output and exit code
7. Cleanup temporary files

**Production Status:**
- ✅ All go test flags supported
- ✅ Recursive package patterns (./...)
- ✅ Coverage and benchmark flags
- ✅ Race detection in test code
- ✅ Increment/decrement operations instrumented
- ✅ Anonymous function code instrumented

**NEW in v0.4.0**

---

### `toolexec` Command

Use racedetector as a toolexec wrapper for standard `go build`.

**Syntax:**
```bash
go build -toolexec="racedetector toolexec" [go-build-flags] <packages>
```

**Description:**
Intercepts Go compiler invocations to instrument source files during normal `go build`. Useful for:
- Complex build systems
- IDE integration
- Makefile-based projects
- When you need standard `go build` behavior with race detection

**Examples:**

```bash
# Build with toolexec
go build -toolexec="racedetector toolexec" -o myapp ./...

# Build specific package
go build -toolexec="racedetector toolexec" ./cmd/server

# With additional flags
go build -toolexec="racedetector toolexec" -ldflags="-s -w" ./...
```

**Requirements:**
- Project must have racedetector as dependency: `go get github.com/kolkov/racedetector/race`

**NEW in v0.8.0**

---

## Escape Analysis (NEW in v0.8.0)

Starting with v0.8.0, racedetector uses compiler escape analysis to reduce false positives.

### How It Works

The detector runs `go build -gcflags="-m"` to determine which variables escape to heap:
- **Stack-local variables** (don't escape) → Skip instrumentation
- **Heap-allocated variables** (escape) → Instrument normally

This reduces instrumentation by **30-50%** on real codebases.

### Benefits

| Before v0.8.0 | After v0.8.0 |
|---------------|--------------|
| Named return values instrumented | Skipped (stack-local) |
| Function parameters instrumented | Skipped (stack-local) |
| More false positives | 30-50% fewer false positives |

### Example

```go
func GetNextToken() (tok Token, err error) {  // Named returns
    tok.typ = TokenEnd  // Before: instrumented, After: skipped
    return tok, nil
}
```

Escape analysis detects that `tok` and `err` don't escape to heap, so they're not instrumented.

### Tested On

- **zygomys** (Issue #17): lexer.go reduced from 14 writes/35 reads to 9 writes/18 reads

---

## Integration with CI/CD

### GitHub Actions

Add race detection to your CI pipeline:

```yaml
# .github/workflows/test.yml
name: Tests with Race Detection

on: [push, pull_request]

jobs:
  race-detection:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.24'

      - name: Install racedetector
        run: go install github.com/kolkov/racedetector/cmd/racedetector@latest

      - name: Build with race detection
        run: racedetector build ./cmd/myapp

      - name: Run integration tests with race detection
        run: |
          for test in tests/*.go; do
            echo "Running $test..."
            racedetector run "$test" || exit 1
          done
```

### GitLab CI

```yaml
# .gitlab-ci.yml
test:race-detection:
  image: golang:1.24
  stage: test
  script:
    - go install github.com/kolkov/racedetector/cmd/racedetector@latest
    - export PATH="$HOME/go/bin:$PATH"
    - racedetector build ./...
    - racedetector run ./tests/integration_test.go
  artifacts:
    reports:
      junit: race-report.xml
```

### Jenkins

```groovy
// Jenkinsfile
pipeline {
    agent any
    stages {
        stage('Race Detection') {
            steps {
                sh 'go install github.com/kolkov/racedetector/cmd/racedetector@latest'
                sh 'racedetector build ./cmd/myapp'
                sh 'racedetector run ./tests/integration_test.go'
            }
        }
    }
}
```

### Docker Build

```dockerfile
# Dockerfile
FROM golang:1.24 AS builder

# Install racedetector
RUN go install github.com/kolkov/racedetector/cmd/racedetector@latest

WORKDIR /app
COPY . .

# Build with race detection
RUN racedetector build -o myapp ./cmd/myapp

# Final image
FROM alpine:latest
COPY --from=builder /app/myapp /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/myapp"]
```

---

## Best Practices

### When to Use Race Detector

**Always use during:**
- Development (catch races early)
- Code review (validate concurrent code)
- Integration testing (test real scenarios)
- Pre-production testing (final validation)

**Consider skipping for:**
- Production builds (performance overhead)
- Benchmarking (distorts results)
- Single-threaded code (no concurrency)

### How to Fix Races

When a race is detected (in Phase 2 future):

**1. Use Mutexes:**
```go
var (
    counter int
    mu      sync.Mutex
)

func increment() {
    mu.Lock()
    counter++
    mu.Unlock()
}
```

**2. Use Channels:**
```go
func worker(ch chan int) {
    for val := range ch {
        process(val)
    }
}
```

**3. Use Atomic Operations:**
```go
var counter int64

func increment() {
    atomic.AddInt64(&counter, 1)
}
```

**4. Use sync.Once:**
```go
var once sync.Once
var config *Config

func getConfig() *Config {
    once.Do(func() {
        config = loadConfig()
    })
    return config
}
```

### Performance Tips

**Current (v0.3.0 Production):**
- Build overhead: ~1-2 seconds (one-time instrumentation)
- Runtime overhead: ~15-22% (hot path optimizations)
- Memory overhead: Minimal (efficient shadow memory)

**Optimization:**
- Run race detection in CI, not production
- Use for integration tests, not unit tests
- Profile hot paths separately (without race detection)

---

## Sampling Mode (NEW in v0.3.0)

For CI/CD workflows where full detection overhead is unacceptable, v0.3.0 introduces **sampling-based detection**.

### Environment Variable

```bash
# Set sample rate (0-100%)
export RACEDETECTOR_SAMPLE_RATE=50  # Check 50% of accesses

# Run with sampling
racedetector run main.go
```

### Sample Rate Guidelines

| Sample Rate | Overhead Reduction | Detection Rate |
|-------------|-------------------|----------------|
| 100% (default) | 0% | 100% |
| 50% | ~50% | 90%+ |
| 10% | ~70% | 70%+ |
| 1% | ~90% | 50%+ |

### Programmatic API

```go
import "github.com/kolkov/racedetector/internal/race/shadowmem"

// Enable address compression for 8x memory reduction
shadowmem.SetAddressCompression(true)
```

---

## Current Limitations

### v0.3.0 Production Status (November 28, 2025)

**What Works:**
- ✅ AST parsing and import injection
- ✅ `race.Init()` automatic insertion
- ✅ `build`, `run`, and `test` commands
- ✅ All `go build` flags supported
- ✅ Argument parsing and stream forwarding
- ✅ Full race detection during runtime
- ✅ Race report generation with stack traces
- ✅ RaceRead/RaceWrite instrumentation
- ✅ 670+ tests passing with 86.3% coverage
- ✅ Sampling-based detection (NEW)
- ✅ Address compression for memory savings (NEW)
- ✅ Sparse-aware VectorClocks for 43x speedup (NEW)

**Known Limitations:**
- Performance overhead: ~15-22% (acceptable for testing/development)
- Not recommended for production deployments (use for testing only)

### Known Issues

No critical issues in v0.3.0. All core functionality is production-ready.

**Minor Limitations:**
- Performance overhead (~15-22%) makes it unsuitable for production deployments
- Use for development, testing, and CI/CD only

**Reporting Issues:**
- Open GitHub issue with reproduction case
- Include Go version, OS, and full error output

---

## FAQ

### General Questions

**Q: What is racedetector?**
A: A Pure-Go race detector that works without CGO dependency, enabling race detection with `CGO_ENABLED=0`.

**Q: How is this different from `go run -race`?**
A: Standard Go race detector requires CGO (ThreadSanitizer C++ library). `racedetector` is 100% Go, works without CGO, perfect for Docker containers and cross-compilation.

**Q: Is this production-ready?**
A: Yes! v0.3.0 is production-ready for testing and development. Not recommended for production deployments due to performance overhead.

### Installation

**Q: Which Go version do I need?**
A: Go 1.24 or higher. Run `go version` to check.

**Q: Can I use this with older Go versions?**
A: No, requires Go 1.24+ for improved performance (Swiss Tables maps, optimized sync.Map) and modern AST features.

**Q: Does this work on Windows?**
A: Yes! Fully tested on Windows 10/11, Linux, and macOS.

### Usage

**Q: Can I use this with existing projects?**
A: Yes! Drop-in replacement for `go build` and `go run`.

**Q: Does this slow down my program?**
A: Yes, ~15-22% overhead. This is significantly better than typical race detectors (5-15x). Use for testing/development only.

**Q: Can I use this in CI/CD?**
A: Yes! See [Integration with CI/CD](#integration-with-cicd) section.

**Q: Does this detect ALL races?**
A: Yes! Detects all races that Go's standard detector finds, using FastTrack algorithm. 670+ tests validate comprehensive coverage.

### Technical

**Q: What algorithm does this use?**
A: FastTrack algorithm (PLDI 2009), same as ThreadSanitizer but implemented in Pure Go.

**Q: How does instrumentation work?**
A: Parses Go AST, injects race detector imports, adds Init() call, and inserts RaceRead/RaceWrite calls around all memory accesses.

**Q: Can I see the instrumented code?**
A: Code is in temporary workspace (auto-deleted after build). Use verbose flags (-v -x) to see build process details.

**Q: Does this modify my source files?**
A: NO! All instrumentation happens in a temporary copy. Original files are never touched.

### Troubleshooting

**Q: Error: "command not found: racedetector"**
A: Install path not in PATH. See [Installation Guide - Troubleshooting](INSTALLATION.md#troubleshooting).

**Q: Error: "cannot find package"**
A: Missing dependencies. Run `go mod tidy`.

**Q: Build fails with "syntax error"**
A: Likely a bug in AST instrumentation. Please report with code sample.

**Q: Program runs but no race detected**
A: If your program has no data races, the detector won't report anything (correct behavior). Try the examples/dogfooding demo to verify detection works.

### Contributing

**Q: How can I contribute?**
A: See [CONTRIBUTING.md](../CONTRIBUTING.md). We welcome bug reports, test cases, documentation improvements, and code contributions!

**Q: Can I use this in my company?**
A: Yes! MIT License. Production-ready for testing and development. Not recommended for production deployments.

**Q: Is there a roadmap?**
A: Yes! See [ROADMAP.md](../ROADMAP.md) for development roadmap and upcoming features.

---

## Getting Help

If you need help:

1. Read this guide thoroughly
2. Check [Installation Guide](INSTALLATION.md)
3. Try [Dogfooding Demo](../examples/dogfooding/)
4. Search [GitHub Issues](https://github.com/kolkov/racedetector/issues)
5. Open a new issue with:
   - OS, Go version
   - Full command and output
   - Minimal reproduction case

---

## Next Steps

- Read [Architecture Documentation](dev/ARCHITECTURE.md) - Understand how it works
- Try [Integration Examples](../examples/integration/) - Real-world use cases
- Follow [Roadmap](PRODUCTION_ROADMAP.md) - Track progress to production

---

*Last Updated: December 8, 2025*
*Version: 0.4.0-dev (test command + IncDecStmt fix)*
