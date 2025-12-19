# Installation Guide - Pure-Go Race Detector

This guide provides step-by-step instructions for installing the `racedetector` tool.

## Prerequisites

- **Go 1.24 or higher** - Starting with v0.3.2, Go 1.24+ is required for better performance
- **Git** - For installing from source

### Verify Prerequisites

```bash
# Check Go version (must be 1.24+)
go version

# Check Git
git --version
```

If you don't have Go installed, visit [https://go.dev/dl/](https://go.dev/dl/)

---

## Windows Antivirus Note

**Windows Defender may flag racedetector as a false positive.** This is common for code instrumentation tools.

### Why this happens

Race detectors and similar tools:
- Modify source code at build time (inserting instrumentation hooks)
- Generate new executables from instrumented code
- These behaviors trigger heuristic antivirus detection

### Workarounds

**Option 1: Add exclusion for Go bin directory**
```
Windows Security → Virus & threat protection → Manage settings →
Exclusions → Add exclusion → Add folder: %USERPROFILE%\go\bin
```

**Option 2: Build from source** (see Method 2 below)

**Option 3: Use WSL2** - Linux binaries don't trigger Windows Defender

This is a known issue ([#7](https://github.com/kolkov/racedetector/issues/7)) and we're investigating code signing solutions.

---

## Installation Methods

### Method 1: Install via `go install` (Recommended)

This is the easiest way to install `racedetector` for end users.

```bash
# Install latest stable version
go install github.com/kolkov/racedetector/cmd/racedetector@latest

# Verify installation
racedetector --version
```

**Expected output:**
```
racedetector version 0.3.2
```

**Installation location:**
- Linux/Mac: `$HOME/go/bin/racedetector`
- Windows: `%USERPROFILE%\go\bin\racedetector.exe`

**Note:** Make sure `$GOPATH/bin` (or `$HOME/go/bin`) is in your `PATH`.

### Method 2: Build from Source

For developers who want to modify the tool or contribute:

```bash
# Clone repository
git clone https://github.com/kolkov/racedetector.git
cd racedetector

# Build the tool
go build -o racedetector ./cmd/racedetector

# Move to PATH (optional)
# Linux/Mac:
sudo mv racedetector /usr/local/bin/

# Windows:
# Move racedetector.exe to a directory in your PATH
```

**Verify installation:**
```bash
racedetector --version
```

### Method 3: Download Pre-built Binary

Pre-built binaries are available in GitHub Releases:

- Linux (amd64, arm64)
- macOS (amd64, arm64/M1)
- Windows (amd64)

Visit [Releases](https://github.com/kolkov/racedetector/releases) and download for your platform.

---

## Verification

After installation, verify the tool is working correctly:

```bash
# Check version
racedetector --version

# Show help
racedetector --help

# Test with a simple program
cat > test_race.go <<'EOF'
package main

import "fmt"

func main() {
    var x int
    go func() { x = 1 }()
    go func() { x = 2 }()
    fmt.Println("Done")
}
EOF

# Build with race detection
racedetector build test_race.go

# Clean up
rm test_race.go test_race
```

If all commands succeed, installation is complete!

---

## Troubleshooting

### Issue: `command not found: racedetector`

**Problem:** The installation directory is not in your PATH.

**Solution (Linux/Mac):**
```bash
# Add to ~/.bashrc or ~/.zshrc
export PATH="$HOME/go/bin:$PATH"

# Reload shell
source ~/.bashrc  # or source ~/.zshrc
```

**Solution (Windows):**
1. Open System Properties > Environment Variables
2. Add `%USERPROFILE%\go\bin` to your PATH
3. Restart Command Prompt

### Issue: `package github.com/kolkov/racedetector: not found`

**Problem:** Repository is not yet public or URL is incorrect.

**Solution:**
- For development: Clone repository and use Method 2 (build from source)
- For future: Wait for official release on GitHub

### Issue: `go: go.mod file not found`

**Problem:** Running install command from wrong directory.

**Solution:**
```bash
# Don't run from inside a Go module unless you want to install a specific version
cd $HOME  # or any directory without go.mod
go install github.com/kolkov/racedetector/cmd/racedetector@latest
```

### Issue: Build fails with `cannot find package`

**Problem:** Missing dependencies or outdated Go version.

**Solution:**
```bash
# Update Go modules
go get -u ./...
go mod tidy

# Rebuild
go build -o racedetector ./cmd/racedetector
```

### Issue: Windows - `Access is denied` when moving to Program Files

**Problem:** Insufficient permissions.

**Solution:**
1. Open Command Prompt as Administrator
2. Or move to `%USERPROFILE%\AppData\Local\Programs\` instead

---

## Updating

### Update via `go install`

```bash
# Install latest version
go install github.com/kolkov/racedetector/cmd/racedetector@latest

# Verify new version
racedetector --version
```

### Update from source

```bash
cd racedetector
git pull origin main
go build -o racedetector ./cmd/racedetector
```

---

## Uninstallation

### Remove via `go clean`

```bash
# Remove installed binary
go clean -i github.com/kolkov/racedetector/cmd/racedetector

# Remove module cache (optional)
go clean -modcache
```

### Manual removal

```bash
# Linux/Mac
rm $(which racedetector)

# Windows
# Delete from %USERPROFILE%\go\bin\racedetector.exe
```

---

## Development Setup

For contributors who want to work on the codebase:

```bash
# Clone repository
git clone https://github.com/kolkov/racedetector.git
cd racedetector

# Install dependencies
go mod download

# Run tests
go test ./...

# Build
go build -o racedetector ./cmd/racedetector

# Run dogfooding demo
cd examples/dogfooding
bash demo.sh  # Linux/Mac
demo.bat      # Windows
```

---

## Platform-Specific Notes

### Linux

- Tested on Ubuntu 20.04+, Debian 11+, Fedora 36+
- Requires GLIBC 2.27+ (standard on modern distributions)
- ARM64 support available

### macOS

- Tested on macOS 11 (Big Sur) and later
- Apple Silicon (M1/M2) fully supported
- Intel (x86_64) supported

### Windows

- Tested on Windows 10 and Windows 11
- Requires 64-bit Windows
- WSL2 (Windows Subsystem for Linux) supported

---

## Next Steps

After installation:

1. Read [USAGE_GUIDE.md](USAGE_GUIDE.md) - Learn how to use the tool
2. Try [examples/dogfooding/](../examples/dogfooding/) - See dogfooding demo
3. Read [README.md](../README.md) - Understand project goals

---

## Getting Help

If you encounter issues:

1. Check [Troubleshooting](#troubleshooting) section above
2. Read [FAQ](USAGE_GUIDE.md#faq) in Usage Guide
3. Search [GitHub Issues](https://github.com/kolkov/racedetector/issues)
4. Open a new issue with:
   - OS and Go version (`go version`)
   - Full error message
   - Steps to reproduce

---

*Last Updated: December 19, 2025*
*Version: 0.8.4*
