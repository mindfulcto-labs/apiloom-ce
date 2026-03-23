package wasm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/plugin/wasm"
)

// minimalWASM returns a minimal valid WASM module binary that exports a _start function.
// Equivalent WAT:
//
//	(module (func (export "_start")))
//
// Byte layout validated against wazero v1.11.0.
func minimalWASM() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6D, // magic
		0x01, 0x00, 0x00, 0x00, // version
		// Type section (id=1): size=4, count=1, func type () -> ()
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		// Function section (id=3): size=2, count=1, type_idx=0
		0x03, 0x02, 0x01, 0x00,
		// Export section (id=7): size=10, count=1, name_len=6, "_start", kind=func(0), idx=0
		0x07, 0x0A, 0x01, 0x06, 0x5F, 0x73, 0x74, 0x61, 0x72, 0x74, 0x00, 0x00,
		// Code section (id=10): size=4, count=1, body_size=2, local_count=0, end=0x0B
		0x0A, 0x04, 0x01, 0x02, 0x00, 0x0B,
	}
}

func TestSandbox_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wasm      []byte
		input     []byte
		wantErr   bool
		errTarget error
	}{
		{
			name:    "invalid WASM bytes rejected",
			wasm:    []byte("not a wasm module"),
			input:   nil,
			wantErr: true,
		},
		{
			name:    "empty bytes rejected",
			wasm:    []byte{},
			input:   nil,
			wantErr: true,
		},
		{
			name:    "valid minimal WASM executes successfully",
			wasm:    minimalWASM(),
			input:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			caps := wasm.NewCapabilitySet(nil)
			sb, err := wasm.NewSandbox(ctx, caps, 1_000_000)
			require.NoError(t, err)
			defer sb.Close(ctx) //nolint:errcheck

			_, err = sb.Execute(ctx, tt.wasm, tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errTarget != nil {
					assert.True(t, errors.Is(err, tt.errTarget))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSandbox_FuelExhausted(t *testing.T) {
	t.Parallel()

	// Infinite loop WASM: (module (func (export "_start") (loop (br 0))))
	// Byte layout validated against wazero v1.11.0.
	infiniteLoopWASM := []byte{
		0x00, 0x61, 0x73, 0x6D, // magic
		0x01, 0x00, 0x00, 0x00, // version
		// Type section (id=1): size=4, count=1, func type () -> ()
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		// Function section (id=3): size=2, count=1, type_idx=0
		0x03, 0x02, 0x01, 0x00,
		// Export section (id=7): size=10, count=1, "_start" -> func 0
		0x07, 0x0A, 0x01, 0x06, 0x5F, 0x73, 0x74, 0x61, 0x72, 0x74, 0x00, 0x00,
		// Code section (id=10): size=9, count=1, body_size=7
		// 0x00=local_count, 0x03 0x40=loop void, 0x0C 0x00=br 0, 0x0B=end loop, 0x0B=end func
		0x0A, 0x09, 0x01, 0x07, 0x00, 0x03, 0x40, 0x0C, 0x00, 0x0B, 0x0B,
	}

	ctx := context.Background()
	caps := wasm.NewCapabilitySet(nil)
	// Very short timeout to trigger fuel exhaustion quickly.
	sb, err := wasm.NewSandbox(ctx, caps, 1000)
	require.NoError(t, err)
	defer sb.Close(ctx) //nolint:errcheck

	_, err = sb.Execute(ctx, infiniteLoopWASM, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, wasm.ErrFuelExhausted), "expected ErrFuelExhausted, got: %v", err)
}

func TestSandbox_Close(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	caps := wasm.NewCapabilitySet(nil)
	sb, err := wasm.NewSandbox(ctx, caps, 1_000_000)
	require.NoError(t, err)
	err = sb.Close(ctx)
	assert.NoError(t, err)
}
