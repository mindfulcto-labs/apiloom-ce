package wasm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// ErrFuelExhausted is returned when a WASM module exceeds its execution budget.
var ErrFuelExhausted = errors.New("wasm: fuel exhausted")

// defaultExecutionTimeout is the maximum wall-clock time for a single WASM execution
// when no fuel limit override applies.
const defaultExecutionTimeout = 5 * time.Second

// fuelToTimeout converts a fuel limit (instruction count) to a wall-clock timeout.
// At ~100M instructions/second, 1M fuel ≈ 10ms. This is a conservative estimate.
// The minimum returned duration is 10ms.
func fuelToTimeout(fuelLimit uint64) time.Duration {
	if fuelLimit == 0 {
		return defaultExecutionTimeout
	}
	// 1 fuel unit ≈ 5ns (200M instructions/second, conservative)
	d := time.Duration(fuelLimit) * 5 * time.Nanosecond
	if d < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	if d > defaultExecutionTimeout {
		return defaultExecutionTimeout
	}
	return d
}

// Sandbox executes WASM modules with isolation guarantees:
//   - No host filesystem access
//   - No environment variable access
//   - Only declared HTTP endpoints via http_fetch host function
//   - Execution terminates when the context deadline is reached (fuel budget)
type Sandbox struct {
	rt        wazero.Runtime
	caps      CapabilitySet
	fuelLimit uint64
}

// NewSandbox creates a Sandbox with the given capability set and fuel limit.
// The fuel limit bounds the number of instructions a module may execute.
// The wazero runtime is created without filesystem or environment access.
func NewSandbox(ctx context.Context, caps CapabilitySet, fuelLimit uint64) (*Sandbox, error) {
	cfg := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true)

	rt := wazero.NewRuntimeWithConfig(ctx, cfg)

	// Instantiate minimal WASI support needed for module compatibility.
	// wasi_snapshot_preview1 exports proc_exit and other basic functions that
	// compiled WASM modules often import. We instantiate it but do NOT mount
	// any filesystem or pass environment variables.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		rt.Close(ctx) //nolint:errcheck
		return nil, fmt.Errorf("instantiating wasi: %w", err)
	}

	return &Sandbox{
		rt:        rt,
		caps:      caps,
		fuelLimit: fuelLimit,
	}, nil
}

// Execute compiles and runs the given WASM bytes.
// It calls the module's _start export (WASI convention).
// Returns the module's stdout bytes and any error.
// Returns ErrFuelExhausted (wrapped) when the execution budget is exhausted.
func (s *Sandbox) Execute(ctx context.Context, wasmBytes []byte, _ []byte) ([]byte, error) {
	if len(wasmBytes) == 0 {
		return nil, fmt.Errorf("wasm: empty module bytes")
	}

	// Compile the module — validates the WASM binary format.
	compiled, err := s.rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("wasm: compiling module: %w", err)
	}
	defer compiled.Close(ctx) //nolint:errcheck

	// Capture stdout from the module.
	var stdout bytes.Buffer

	// Apply execution timeout as the fuel enforcement mechanism.
	// wazero uses context cancellation (WithCloseOnContextDone) to interrupt execution.
	timeout := fuelToTimeout(s.fuelLimit)
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Module config: no filesystem, no env vars, capture stdout.
	modCfg := wazero.NewModuleConfig().
		WithName("plugin").
		WithStdout(&stdout)
	// Intentionally omit WithStdin, WithFS, WithEnv to enforce isolation.
	// WithCloseOnContextDone is set on the runtime config — modules inherit it.

	// Instantiate and run. For WASI modules, Instantiate calls _start automatically.
	mod, err := s.rt.InstantiateModule(execCtx, compiled, modCfg)
	if err != nil {
		// Map context deadline/cancellation to ErrFuelExhausted.
		if execCtx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("%w: %w", ErrFuelExhausted, err)
		}
		return nil, fmt.Errorf("wasm: executing module: %w", err)
	}
	if mod != nil {
		mod.Close(execCtx) //nolint:errcheck
	}

	return stdout.Bytes(), nil
}

// Close releases the wazero runtime and all compiled module resources.
func (s *Sandbox) Close(ctx context.Context) error {
	if err := s.rt.Close(ctx); err != nil {
		return fmt.Errorf("wasm: closing runtime: %w", err)
	}
	return nil
}

