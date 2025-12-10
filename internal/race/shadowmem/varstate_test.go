package shadowmem

import (
	"testing"
	"unsafe"

	"github.com/kolkov/racedetector/internal/race/epoch"
)

// TestVarStateSize verifies that VarState has expected size.
// Phase 3: 24 bytes (W + mu + readEpoch + readClock pointer).
// v0.2.0 Task 3 (SmartTrack): 40 bytes (adds exclusiveWriter int64 + writeCount uint32 + padding).
// v0.2.0 Task 4 (64-bit Epoch): 48 bytes (W changed from uint32 to uint64, adds 8 bytes).
// v0.2.0 Task 6 (Stack Depot): 64 bytes (adds writeStackHash uint64 + readStackHash uint64).
// v0.3.0 P1 (Enhanced Read-Shared): 96 bytes (readEpoch → readEpochs[4] + readerCount uint8).
// v0.3.0 Lock-Free: 112 bytes (W/exclusiveWriter/writePC/readPC become atomic types with padding).
// Trade-off: 112 bytes per VarState BUT lock-free hot path (2-5ns vs 20-50ns mutex).
func TestVarStateSize(t *testing.T) {
	// v0.3.0 Lock-Free: atomic.Uint64 W(24) + atomic.Int64 exclusiveWriter(24) + atomic.Uintptr writePC(24)
	//       + atomic.Uintptr readPC(24) + mu(8) + readEpochs[4](32) + readerCount(1) + padding
	//       + readClock(8) + writeCount(4) + writeStackHash(8) + readStackHash(8) = 112
	// Note: Atomic types have additional padding for alignment, increasing from 96 to 112 bytes.
	const expectedSize = 112
	actualSize := unsafe.Sizeof(VarState{})

	if actualSize != expectedSize {
		t.Errorf("VarState size = %d bytes, want %d bytes (v0.3.0 lock-free with atomic fields)", actualSize, expectedSize)
	}

	t.Logf("VarState size: %d bytes (v0.3.0 Lock-Free with atomic hot-path fields)", actualSize)
}

// TestVarStateNewZero verifies that NewVarState creates a zero-initialized state.
func TestVarStateNewZero(t *testing.T) {
	vs := NewVarState()

	if vs == nil {
		t.Fatal("NewVarState() returned nil")
	}

	// Both W and readEpoch should be zero.
	if vs.GetW() != 0 {
		t.Errorf("NewVarState().W = %v, want 0", vs.GetW())
	}
	if vs.GetReadEpoch() != 0 {
		t.Errorf("NewVarState().GetReadEpoch() = %v, want 0", vs.GetReadEpoch())
	}

	// Should not be promoted.
	if vs.IsPromoted() {
		t.Error("NewVarState() should not be promoted")
	}

	// Verify epochs decode to zero TID and clock.
	wTID, wClock := vs.GetW().Decode()
	rTID, rClock := vs.GetReadEpoch().Decode()

	if wTID != 0 || wClock != 0 {
		t.Errorf("NewVarState().W decoded = (tid=%d, clock=%d), want (0, 0)", wTID, wClock)
	}
	if rTID != 0 || rClock != 0 {
		t.Errorf("NewVarState().GetReadEpoch() decoded = (tid=%d, clock=%d), want (0, 0)", rTID, rClock)
	}

	t.Logf("NewVarState() correctly initialized: W=%s R=%s", vs.GetW(), vs.GetReadEpoch())
}

// TestVarStateReset verifies that Reset zeros both W and readEpoch fields.
func TestVarStateReset(t *testing.T) {
	vs := NewVarState()

	// Set both W and readEpoch to non-zero values.
	vs.SetW(epoch.NewEpoch(5, 100))
	vs.SetReadEpoch(epoch.NewEpoch(3, 50))

	// Verify they were set.
	if vs.GetW() == 0 || vs.GetReadEpoch() == 0 {
		t.Fatalf("Setup failed: W=%v readEpoch=%v, expected non-zero", vs.GetW(), vs.GetReadEpoch())
	}

	// Reset should zero both fields.
	vs.Reset()

	if vs.GetW() != 0 {
		t.Errorf("After Reset(), W = %v, want 0", vs.GetW())
	}
	if vs.GetReadEpoch() != 0 {
		t.Errorf("After Reset(), GetReadEpoch() = %v, want 0", vs.GetReadEpoch())
	}
	if vs.IsPromoted() {
		t.Error("After Reset(), should not be promoted")
	}

	t.Logf("Reset() correctly zeroed state: W=%s R=%s", vs.GetW(), vs.GetReadEpoch())
}

// TestVarStateReadWrite verifies that W and R epochs can be set and read.
func TestVarStateReadWrite(t *testing.T) {
	tests := []struct {
		name     string
		wTID     uint16
		wClock   uint32
		rTID     uint16
		rClock   uint32
		wantWStr string // Expected W.String() format.
		wantRStr string // Expected R.String() format.
	}{
		{
			name:     "simple write and read",
			wTID:     5,
			wClock:   100,
			rTID:     3,
			rClock:   50,
			wantWStr: "100@5",
			wantRStr: "50@3",
		},
		{
			name:     "same thread write and read",
			wTID:     7,
			wClock:   200,
			rTID:     7,
			rClock:   199,
			wantWStr: "200@7",
			wantRStr: "199@7",
		},
		{
			name:     "zero epochs",
			wTID:     0,
			wClock:   0,
			rTID:     0,
			rClock:   0,
			wantWStr: "0@0",
			wantRStr: "0@0",
		},
		{
			name:     "max thread ID (255)",
			wTID:     255,
			wClock:   1000,
			rTID:     255,
			rClock:   999,
			wantWStr: "1000@255",
			wantRStr: "999@255",
		},
		{
			name:     "large clock values",
			wTID:     1,
			wClock:   0xFFFFFF, // Max 24-bit clock.
			rTID:     2,
			rClock:   0xFFFFFE,
			wantWStr: "16777215@1", // 0xFFFFFF = 16777215 decimal.
			wantRStr: "16777214@2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := NewVarState()

			// Set W and readEpoch.
			vs.SetW(epoch.NewEpoch(tt.wTID, uint64(tt.wClock)))
			vs.SetReadEpoch(epoch.NewEpoch(tt.rTID, uint64(tt.rClock)))

			// Verify W epoch.
			wTID, wClock := vs.GetW().Decode()
			if wTID != tt.wTID {
				t.Errorf("W.TID = %d, want %d", wTID, tt.wTID)
			}
			if wClock != uint64(tt.wClock) {
				t.Errorf("W.Clock = %d, want %d", wClock, tt.wClock)
			}

			// Verify readEpoch.
			rTID, rClock := vs.GetReadEpoch().Decode()
			if rTID != tt.rTID {
				t.Errorf("GetReadEpoch().TID = %d, want %d", rTID, tt.rTID)
			}
			if rClock != uint64(tt.rClock) {
				t.Errorf("GetReadEpoch().Clock = %d, want %d", rClock, tt.rClock)
			}

			// Verify String() output.
			wStr := vs.GetW().String()
			if wStr != tt.wantWStr {
				t.Errorf("W.String() = %q, want %q", wStr, tt.wantWStr)
			}
			rStr := vs.GetReadEpoch().String()
			if rStr != tt.wantRStr {
				t.Errorf("GetReadEpoch().String() = %q, want %q", rStr, tt.wantRStr)
			}

			t.Logf("VarState: W=%s R=%s", vs.GetW(), vs.GetReadEpoch())
		})
	}
}

// TestVarStateString verifies the String() method's debug output format.
func TestVarStateString(t *testing.T) {
	// v0.3.0 Enhanced Read-Shared: String format changed to "W:x@y R:[epochs...]"
	tests := []struct {
		name string
		vs   func() *VarState
		want string
	}{
		{
			name: "zero state",
			vs: func() *VarState {
				return &VarState{}
			},
			want: "W:0@0 R:[]", // v0.3.0: Empty reader list
		},
		{
			name: "write epoch set",
			vs: func() *VarState {
				vs := NewVarState()
				vs.SetW(epoch.NewEpoch(5, 100))
				return vs
			},
			want: "W:100@5 R:[]", // v0.3.0: Empty reader list
		},
		{
			name: "read epoch set",
			vs: func() *VarState {
				vs := NewVarState()
				vs.SetReadEpoch(epoch.NewEpoch(3, 50))
				return vs
			},
			want: "W:0@0 R:[50@3]", // v0.3.0: Single reader in brackets
		},
		{
			name: "both epochs set",
			vs: func() *VarState {
				vs := NewVarState()
				vs.SetW(epoch.NewEpoch(5, 100))
				vs.SetReadEpoch(epoch.NewEpoch(3, 50))
				return vs
			},
			want: "W:100@5 R:[50@3]", // v0.3.0: Single reader in brackets
		},
		{
			name: "same thread",
			vs: func() *VarState {
				vs := NewVarState()
				vs.SetW(epoch.NewEpoch(7, 200))
				vs.SetReadEpoch(epoch.NewEpoch(7, 199))
				return vs
			},
			want: "W:200@7 R:[199@7]", // v0.3.0: Single reader in brackets
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := tt.vs()
			got := vs.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			t.Logf("String() output: %s", got)
		})
	}
}

// TestVarStateZeroValue verifies that a zero VarState (no initialization) works correctly.
func TestVarStateZeroValue(t *testing.T) {
	var vs VarState // Zero value, not initialized with NewVarState().

	// Should be equivalent to NewVarState().
	if vs.GetW() != 0 {
		t.Errorf("Zero VarState.W = %v, want 0", vs.GetW())
	}
	if vs.GetReadEpoch() != 0 {
		t.Errorf("Zero VarState.GetReadEpoch() = %v, want 0", vs.GetReadEpoch())
	}
	if vs.IsPromoted() {
		t.Error("Zero VarState should not be promoted")
	}

	// String should work on zero value.
	// v0.3.0: Format changed to "W:x@y R:[epochs...]"
	str := vs.String()
	if str != "W:0@0 R:[]" {
		t.Errorf("Zero VarState.String() = %q, want %q", str, "W:0@0 R:[]")
	}

	t.Logf("Zero VarState works correctly: %s", vs.String())
}

// TestVarStateResetNoAlloc verifies that Reset() does not allocate.
func TestVarStateResetNoAlloc(t *testing.T) {
	vs := NewVarState()
	vs.SetW(epoch.NewEpoch(5, 100))
	vs.SetReadEpoch(epoch.NewEpoch(3, 50))

	// Measure allocations during Reset().
	allocs := testing.AllocsPerRun(1000, func() {
		vs.Reset()
	})

	if allocs > 0 {
		t.Errorf("Reset() allocated %.2f times per call, want 0", allocs)
	}

	t.Logf("Reset() allocations: %.2f (correct - zero allocations)", allocs)
}

// BenchmarkVarStateNew benchmarks the cost of NewVarState().
func BenchmarkVarStateNew(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewVarState()
	}
}

// BenchmarkVarStateReset benchmarks the cost of Reset().
// Target: <2ns/op, 0 allocs/op.
func BenchmarkVarStateReset(b *testing.B) {
	vs := NewVarState()
	vs.SetW(epoch.NewEpoch(5, 100))
	vs.SetReadEpoch(epoch.NewEpoch(3, 50))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vs.Reset()
	}
}

// BenchmarkVarStateReadWrite benchmarks the cost of setting W and readEpoch.
func BenchmarkVarStateReadWrite(b *testing.B) {
	vs := NewVarState()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vs.SetW(epoch.NewEpoch(5, uint64(i)))
		vs.SetReadEpoch(epoch.NewEpoch(3, uint64(i)))
	}
}

// BenchmarkVarStateString benchmarks the cost of String() formatting.
// This is not on hot path, but good to know the cost.
func BenchmarkVarStateString(b *testing.B) {
	vs := NewVarState()
	vs.SetW(epoch.NewEpoch(5, 100))
	vs.SetReadEpoch(epoch.NewEpoch(3, 50))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = vs.String()
	}
}
