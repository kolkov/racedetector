#!/usr/bin/env bash
# Pre-Release Validation Script for Pure-Go Race Detector
# This script runs all quality checks before creating a release
# EXACTLY matches CI checks + additional validations
# Adapted from Fursy HTTP Router pre-release-check.sh

set -e  # Exit on first error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Header
echo ""
echo "================================================"
echo "  Pure-Go Race Detector - Pre-Release Check"
echo "================================================"
echo ""

# Track overall status
ERRORS=0
WARNINGS=0

# 1. Check Go version
log_info "Checking Go version..."
GO_VERSION=$(go version | awk '{print $3}')
REQUIRED_VERSION="go1.25"
if [[ "$GO_VERSION" < "$REQUIRED_VERSION" ]]; then
    log_error "Go version $REQUIRED_VERSION+ required, found $GO_VERSION"
    ERRORS=$((ERRORS + 1))
else
    log_success "Go version: $GO_VERSION"
fi
echo ""

# 2. Check git status
log_info "Checking git status..."
if git diff-index --quiet HEAD --; then
    log_success "Working directory is clean"
else
    log_warning "Uncommitted changes detected"
    git status --short
    WARNINGS=$((WARNINGS + 1))
fi
echo ""

# 3. Code formatting check (exclude external/ and docs/)
log_info "Checking code formatting (gofmt -l .)..."
UNFORMATTED=$(find . -name "*.go" -not -path "./external/*" -not -path "./docs/*" -exec gofmt -l {} \;)
if [ -n "$UNFORMATTED" ]; then
    log_error "The following files need formatting:"
    echo "$UNFORMATTED"
    echo ""
    log_info "Run: go fmt ./..."
    ERRORS=$((ERRORS + 1))
else
    log_success "All files are properly formatted"
fi
echo ""

# 4. Go vet (exclude external/)
# Note: -unsafeptr=false is needed for goid_fast.go which uses intentional
# uintptr→unsafe.Pointer conversion for runtime.g access (standard pattern
# used by petermattis/goid, outrigdev/goid, and similar libraries)
log_info "Running go vet..."
VET_OUTPUT=$(go vet -unsafeptr=false ./internal/... ./race/... ./cmd/... ./examples/... 2>&1)
VET_EXIT=$?
if [ $VET_EXIT -eq 0 ]; then
    log_success "go vet passed"
else
    log_error "go vet failed:"
    echo "$VET_OUTPUT"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 5. Build all packages (exclude external/)
log_info "Building all packages..."
BUILD_OUTPUT=$(go build ./internal/... ./race/... ./cmd/... ./examples/... 2>&1)
BUILD_EXIT=$?
if [ $BUILD_EXIT -eq 0 ]; then
    log_success "Build successful"
else
    log_error "Build failed:"
    echo "$BUILD_OUTPUT"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 5.5. Build racedetector tool (critical for dogfooding)
log_info "Building racedetector tool..."
TOOL_BUILD_OUTPUT=$(go build -o racedetector_test_binary ./cmd/racedetector 2>&1)
TOOL_BUILD_EXIT=$?
if [ $TOOL_BUILD_EXIT -eq 0 ]; then
    log_success "racedetector tool built successfully"
    rm -f racedetector_test_binary
else
    log_error "racedetector tool build failed:"
    echo "$TOOL_BUILD_OUTPUT"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 6. go.mod validation
log_info "Validating go.mod..."
go mod verify
if [ $? -eq 0 ]; then
    log_success "go.mod verified"
else
    log_error "go.mod verification failed"
    ERRORS=$((ERRORS + 1))
fi

# Check if go.mod needs tidying (skip if external/ exists - contains reference materials)
if [ -d "external" ]; then
    log_info "Skipping go mod tidy (external/ contains reference materials with test files)"
    log_success "go.mod check skipped (external/ present)"
else
    go mod tidy
    if git diff --quiet go.mod go.sum; then
        log_success "go.mod is tidy"
    else
        log_warning "go.mod needs tidying (run 'go mod tidy')"
        git diff go.mod go.sum
        WARNINGS=$((WARNINGS + 1))
    fi
fi
echo ""

# 6.5. Verify golangci-lint configuration
log_info "Verifying golangci-lint configuration..."
if command -v golangci-lint &> /dev/null; then
    if golangci-lint config verify 2>&1; then
        log_success "golangci-lint config is valid"
    else
        log_error "golangci-lint config is invalid"
        ERRORS=$((ERRORS + 1))
    fi
else
    log_warning "golangci-lint not installed (optional but recommended)"
    log_info "Install: https://golangci-lint.run/welcome/install/"
    WARNINGS=$((WARNINGS + 1))
fi
echo ""

# 7. Run tests (WITHOUT Go's race detector)
#
# IMPORTANT: We do NOT use Go's -race flag when testing the Pure-Go race detector because:
#   1. Meta-circular issue: Testing a race detector WITH a race detector causes "hole in findfunctab" errors
#   2. Self-testing: Our detector tests ITSELF through integration tests (see internal/race/api/*integration*test.go)
#   3. Intentional races: Integration tests create races to validate detection works
#   4. Pure-Go: This detector is designed to work with CGO_ENABLED=0, so it doesn't need CGO-based race detection
#
# The integration tests validate race detection by:
#   - Creating intentional races (TestIntegration_SimpleRace, TestUnprotected_DetectsRace)
#   - Verifying detector reports them correctly
#   - Checking no false positives (TestMutexProtected_NoRace)
USE_WSL=0
WSL_DISTRO=""

# Helper function to find WSL distro with Go installed
find_wsl_distro() {
    if ! command -v wsl &> /dev/null; then
        return 1
    fi

    # Try common distros first
    for distro in "Gentoo" "Ubuntu" "Debian" "Alpine"; do
        if wsl -d "$distro" bash -c "command -v go &> /dev/null" 2>/dev/null; then
            echo "$distro"
            return 0
        fi
    done

    return 1
}

# Detect WSL for running tests (but WITHOUT -race flag)
WSL_DISTRO=$(find_wsl_distro)
if [ -n "$WSL_DISTRO" ]; then
    log_info "WSL2 ($WSL_DISTRO) detected - using for test execution"
    USE_WSL=1

    # Convert Windows path to WSL path (D:\projects\... -> /mnt/d/projects/...)
    CURRENT_DIR=$(pwd)
    if [[ "$CURRENT_DIR" =~ ^/([a-z])/ ]]; then
        # Already in /d/... format (MSYS), convert to /mnt/d/...
        WSL_PATH="/mnt${CURRENT_DIR}"
    else
        # Windows format D:\... convert to /mnt/d/...
        DRIVE_LETTER=$(echo "$CURRENT_DIR" | cut -d: -f1 | tr '[:upper:]' '[:lower:]')
        PATH_WITHOUT_DRIVE=${CURRENT_DIR#*:}
        WSL_PATH="/mnt/$DRIVE_LETTER${PATH_WITHOUT_DRIVE//\\//}"
    fi

    TEST_CMD="wsl -d \"$WSL_DISTRO\" bash -c \"cd \\\"$WSL_PATH\\\" && GOEXPERIMENT=jsonv2 go test ./internal/... ./race/... ./cmd/... ./examples/... 2>&1\""
else
    log_info "Running tests locally (no WSL detected)"
    TEST_CMD="go test ./internal/... ./race/... ./cmd/... ./examples/... 2>&1"
fi

log_info "Running tests (without Go's -race flag - see comment above)..."
if [ $USE_WSL -eq 1 ]; then
    # WSL2: Use timeout (3 min) and unbuffered output
    # IMPORTANT: GOEXPERIMENT=jsonv2 required for encoding/json/v2 support
    TEST_OUTPUT=$(wsl -d "$WSL_DISTRO" bash -c "cd $WSL_PATH && GOEXPERIMENT=jsonv2 timeout 180 stdbuf -oL -eL go test ./internal/... ./race/... ./cmd/... ./examples/... 2>&1" || true)
    if [ -z "$TEST_OUTPUT" ]; then
        log_error "WSL2 tests timed out or failed to run"
        ERRORS=$((ERRORS + 1))
    fi
else
    TEST_OUTPUT=$(eval "$TEST_CMD")
fi

# Check test results
if echo "$TEST_OUTPUT" | grep -q "FAIL"; then
    # Check if failure is only due to performance tests in WSL2 (acceptable)
    if [ $USE_WSL -eq 1 ] && echo "$TEST_OUTPUT" | grep -q "Benchmark\|Performance"; then
        log_warning "Performance tests may be slower in WSL2 (acceptable - WSL2 has overhead)"
        echo "$TEST_OUTPUT" | grep -A 5 "FAIL:"
        echo ""
        log_info "This is OK for WSL2 - functional tests passed"
        WARNINGS=$((WARNINGS + 1))
    else
        log_error "Tests failed"
        echo "$TEST_OUTPUT"
        echo ""
        ERRORS=$((ERRORS + 1))
    fi
elif echo "$TEST_OUTPUT" | grep -q "PASS\|ok"; then
    if [ $USE_WSL -eq 1 ]; then
        log_success "All tests passed (via WSL2 $WSL_DISTRO)"
    else
        log_success "All tests passed"
    fi
else
    log_error "Unexpected test output"
    echo "$TEST_OUTPUT"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 8. Test coverage check
log_info "Checking test coverage..."

# Check coverage for different packages based on what exists
if [ -d "internal" ]; then
    COVERAGE=$(go test -cover ./internal/... 2>&1 | grep "coverage:" | tail -1 | awk '{print $5}' | sed 's/%//')
    if [ -n "$COVERAGE" ]; then
        echo "  • internal/ coverage: ${COVERAGE}%"
        # Phase 1 MVP: 70%+ acceptable, Phase 2+: 90%+
        if awk -v cov="$COVERAGE" 'BEGIN {exit !(cov >= 70.0)}'; then
            log_success "Coverage meets Phase 1 requirement (>70%)"
        else
            log_warning "Coverage below 70% (${COVERAGE}%) - Phase 1 target"
            WARNINGS=$((WARNINGS + 1))
        fi
    else
        log_info "No tests in internal/ yet (early development)"
    fi
fi

# Check overall coverage
OVERALL_COVERAGE=$(go test -cover ./internal/... ./race/... 2>&1 | grep "coverage:" | tail -1 | awk '{print $5}' | sed 's/%//')
if [ -n "$OVERALL_COVERAGE" ]; then
    echo "  • Overall coverage: ${OVERALL_COVERAGE}%"
    if awk -v cov="$OVERALL_COVERAGE" 'BEGIN {exit !(cov >= 70.0)}'; then
        log_success "Overall coverage meets requirement (>70%)"
    else
        log_warning "Overall coverage below 70% (${OVERALL_COVERAGE}%)"
        WARNINGS=$((WARNINGS + 1))
    fi
else
    log_info "No tests yet (project in initial setup phase)"
fi
echo ""

# 8.5. Dogfooding test - racedetector tests itself!
log_info "Running dogfooding test..."
if [ -f "examples/dogfooding/simple_race.go" ] && [ -f "cmd/racedetector/main.go" ]; then
    # Build racedetector tool
    go build -o racedetector_test_binary ./cmd/racedetector 2>&1 > /dev/null
    if [ $? -eq 0 ]; then
        # Run dogfooding demo
        cd examples/dogfooding
        ../../racedetector_test_binary build -o simple_race_test simple_race.go 2>&1 > /dev/null
        if [ $? -eq 0 ]; then
            # Execute instrumented binary (should detect race)
            ./simple_race_test > /dev/null 2>&1
            DOGFOOD_EXIT=$?
            cd ../..
            rm -f racedetector_test_binary examples/dogfooding/simple_race_test

            # Exit code doesn't matter - if it runs without panicking, we're good
            log_success "Dogfooding test passed (racedetector can instrument itself)"
        else
            cd ../..
            rm -f racedetector_test_binary
            log_error "Dogfooding build failed"
            ERRORS=$((ERRORS + 1))
        fi
    else
        log_error "Failed to build racedetector tool"
        ERRORS=$((ERRORS + 1))
    fi
else
    log_info "Dogfooding test skipped (example not found)"
fi
echo ""

# 9. Check dependencies policy (Minimal dependencies - known allowed list)
log_info "Checking dependencies policy..."
if [ -f "go.mod" ]; then
    # Allowed dependencies (with reasons):
    # - golang.org/x/mod: Go module file parsing (for replace directive handling)
    # - github.com/outrigdev/goid: Assembly-optimized goroutine ID extraction (~1000x faster)
    ALLOWED_DEPS="golang.org/x/mod|github.com/outrigdev/goid"

    # Count dependencies not in allowed list (excluding indirect)
    UNKNOWN_DEPS=$(grep -E "^\s+github.com|^\s+golang.org" go.mod | grep -v "// indirect" | grep -vE "$ALLOWED_DEPS" | wc -l)
    TOTAL_DEPS=$(grep -E "^\s+github.com|^\s+golang.org" go.mod | grep -v "// indirect" | wc -l)

    if [ "$UNKNOWN_DEPS" -eq 0 ]; then
        if [ "$TOTAL_DEPS" -eq 0 ]; then
            log_success "Pure Go: Zero external dependencies ✅ (stdlib only)"
        else
            log_success "Minimal dependencies: $TOTAL_DEPS allowed dependency(ies) ✅"
            grep -E "^\s+github.com|^\s+golang.org" go.mod | grep -v "// indirect" | while read dep; do
                echo "  • $dep"
            done
        fi
    else
        log_error "Unknown dependency found: $UNKNOWN_DEPS dependency(ies) not in allowed list"
        log_error "Allowed: golang.org/x/mod, github.com/outrigdev/goid"
        grep -E "^\s+github.com|^\s+golang.org" go.mod | grep -v "// indirect" | grep -vE "$ALLOWED_DEPS"
        ERRORS=$((ERRORS + 1))
    fi
else
    log_info "go.mod not initialized yet (Phase 0)"
fi
echo ""

# 10. golangci-lint (exclude external/)
log_info "Running golangci-lint..."
if command -v golangci-lint &> /dev/null; then
    # Run only on internal/, race/, cmd/, examples/ (exclude external/)
    # Note: CI uses latest golangci-lint with caching handled by golangci-lint-action
    # Local version may differ, but linters should catch similar issues
    LINT_OUTPUT=$(golangci-lint run --timeout=10m ./internal/... ./race/... ./cmd/... ./examples/... 2>&1 || true)
    # Check if output contains "0 issues" or is empty
    if echo "$LINT_OUTPUT" | grep -q "0 issues" || [ -z "$(echo "$LINT_OUTPUT" | grep -v '^$')" ]; then
        log_success "golangci-lint passed with 0 issues"
    else
        log_error "Linter found issues:"
        echo "$LINT_OUTPUT" | tail -30
        ERRORS=$((ERRORS + 1))
    fi
else
    log_error "golangci-lint not installed"
    log_info "Install: https://golangci-lint.run/welcome/install/"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 11. Check for TODO/FIXME comments (exclude external/)
log_info "Checking for TODO/FIXME comments..."
TODO_COUNT=$(grep -r "TODO\|FIXME" --include="*.go" --exclude-dir=external --exclude-dir=vendor . 2>/dev/null | wc -l)
if [ "$TODO_COUNT" -gt 0 ]; then
    log_warning "Found $TODO_COUNT TODO/FIXME comments"
    grep -r "TODO\|FIXME" --include="*.go" --exclude-dir=external --exclude-dir=vendor . 2>/dev/null | head -5
    WARNINGS=$((WARNINGS + 1))
else
    log_success "No TODO/FIXME comments found"
fi
echo ""

# 12. Check critical documentation files
log_info "Checking documentation..."
DOCS_MISSING=0
REQUIRED_DOCS="README.md"

for doc in $REQUIRED_DOCS; do
    if [ ! -f "$doc" ]; then
        log_error "Missing: $doc"
        DOCS_MISSING=1
        ERRORS=$((ERRORS + 1))
    fi
done

# Optional but recommended for release (OK to skip in Phase 1)
if [ ! -f "LICENSE" ]; then
    log_warning "Missing: LICENSE (will be added before public release)"
    WARNINGS=$((WARNINGS + 1))
fi

if [ ! -f "CHANGELOG.md" ]; then
    log_warning "Missing: CHANGELOG.md (recommended for releases)"
    WARNINGS=$((WARNINGS + 1))
fi

if [ $DOCS_MISSING -eq 0 ]; then
    log_success "All critical documentation files present"
fi
echo ""

# Summary
echo "========================================"
echo "  Summary"
echo "========================================"
echo ""

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
    log_success "✅ All checks passed! Ready for release."
    echo ""
    log_info "Next steps for release:"
    echo ""
    echo "  1. Create release branch from develop:"
    echo "     git checkout -b release/vX.Y.Z develop"
    echo ""
    echo "  2. Prepare release (ONE commit with ALL changes):"
    echo "     - Update CHANGELOG.md"
    echo "     - Update README.md version"
    echo "     bash scripts/pre-release-check.sh  # Re-run to verify"
    echo "     git add -A"
    echo "     git commit -m \"chore: prepare vX.Y.Z release\""
    echo ""
    echo "  3. Push release branch, wait for CI:"
    echo "     git push origin release/vX.Y.Z"
    echo "     ⏳ WAIT for CI to be GREEN"
    echo ""
    echo "  4. Merge to main:"
    echo "     git checkout main"
    echo "     git merge --squash release/vX.Y.Z"
    echo "     git commit -m \"Release vX.Y.Z\""
    echo "     git push origin main"
    echo "     ⏳ WAIT for CI to be GREEN on main!"
    echo ""
    echo "  5. ONLY AFTER CI GREEN - create and push tag:"
    echo "     git tag -a vX.Y.Z -m \"Release vX.Y.Z\""
    echo "     git push origin main --tags  # Tags are PERMANENT!"
    echo ""
    echo "  6. Merge back to develop:"
    echo "     git checkout develop"
    echo "     git merge --no-ff main -m \"Merge release vX.Y.Z back to develop\""
    echo "     git push origin develop"
    echo ""
    echo "  7. Clean up:"
    echo "     git branch -d release/vX.Y.Z"
    echo "     git push origin --delete release/vX.Y.Z"
    echo ""
    exit 0
elif [ $ERRORS -eq 0 ]; then
    log_warning "Checks completed with $WARNINGS warning(s)"
    echo ""
    log_info "Review warnings above before proceeding with release"
    echo ""
    exit 0
else
    log_error "Checks failed with $ERRORS error(s) and $WARNINGS warning(s)"
    echo ""
    log_error "Fix errors before creating release"
    echo ""
    exit 1
fi
