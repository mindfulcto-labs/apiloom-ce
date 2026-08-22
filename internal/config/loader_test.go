package config

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// stubConfigLoader implements ConfigLoader for unit tests.
// Thread-safe for concurrent access in race-detector tests.
type stubConfigLoader struct {
	mu     sync.Mutex
	config *domain.GatewayConfig
	err    error
}

func (s *stubConfigLoader) LoadGatewayConfig(_ context.Context) (*domain.GatewayConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return s.config, nil
}

func (s *stubConfigLoader) setConfig(cfg *domain.GatewayConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
}

func (s *stubConfigLoader) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func newTestConfig(version int, routeNames ...string) *domain.GatewayConfig {
	routes := make([]domain.RouteConfig, len(routeNames))
	for i, name := range routeNames {
		routes[i] = domain.RouteConfig{
			Name:     name,
			Config:   json.RawMessage(`{"path":"/"}`),
			Checksum: domain.ComputeChecksum(json.RawMessage(`{"path":"/"}`)),
		}
	}
	return &domain.GatewayConfig{
		Version:  version,
		Routes:   routes,
		LoadedAt: time.Now(),
	}
}

func TestLoaderGet_ReturnsConfigAfterInitialLoad(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig(1, "route-a")
	stub := &stubConfigLoader{config: cfg}
	loader := NewLoader(stub)

	err := loader.InitialLoad(context.Background())
	require.NoError(t, err)

	got := loader.Get()
	require.NotNil(t, got)
	assert.Equal(t, 1, got.Version)
	assert.Len(t, got.Routes, 1)
	assert.Equal(t, "route-a", got.Routes[0].Name)
}

func TestLoaderReload_SwapsToNewConfig(t *testing.T) {
	t.Parallel()

	cfg1 := newTestConfig(1, "route-a")
	cfg2 := newTestConfig(2, "route-a", "route-b")

	stub := &stubConfigLoader{config: cfg1}
	loader := NewLoader(stub)

	require.NoError(t, loader.InitialLoad(context.Background()))
	assert.Len(t, loader.Get().Routes, 1)

	// Change stub to return new config.
	stub.config = cfg2

	require.NoError(t, loader.Reload(context.Background()))

	got := loader.Get()
	assert.Equal(t, 2, got.Version)
	assert.Len(t, got.Routes, 2)
}

func TestLoaderReload_StoreError_PreservesPreviousConfig(t *testing.T) {
	t.Parallel()

	cfg1 := newTestConfig(1, "route-a")
	stub := &stubConfigLoader{config: cfg1}
	loader := NewLoader(stub)

	require.NoError(t, loader.InitialLoad(context.Background()))

	// Make stub return error.
	stub.err = errors.New("connection refused")
	stub.config = nil

	err := loader.Reload(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	// Previous config still accessible.
	got := loader.Get()
	require.NotNil(t, got)
	assert.Equal(t, 1, got.Version)
	assert.Len(t, got.Routes, 1)
}

func TestLoaderReload_ConcurrentGet_NeverNil(t *testing.T) {
	t.Parallel()

	cfg1 := newTestConfig(1, "route-a")
	cfg2 := newTestConfig(2, "route-a", "route-b")

	stub := &stubConfigLoader{config: cfg1}
	loader := NewLoader(stub)
	require.NoError(t, loader.InitialLoad(context.Background()))

	var wg sync.WaitGroup
	ctx := context.Background()

	// Spawn readers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got := loader.Get()
				require.NotNil(t, got, "Get() must never return nil during concurrent reload")
			}
		}()
	}

	// Spawn reloaders.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if j%2 == 0 {
					stub.setConfig(cfg2)
				} else {
					stub.setConfig(cfg1)
				}
				_ = loader.Reload(ctx)
			}
		}()
	}

	wg.Wait()
}

func TestLoaderOnReload_CallbackReceivesOldAndNew(t *testing.T) {
	t.Parallel()

	cfg1 := newTestConfig(1, "route-a")
	cfg2 := newTestConfig(2, "route-a", "route-b")

	stub := &stubConfigLoader{config: cfg1}

	var callbackOld, callbackNew *domain.GatewayConfig
	loader := NewLoader(stub, WithOnReload(func(old, newCfg *domain.GatewayConfig) {
		callbackOld = old
		callbackNew = newCfg
	}))

	require.NoError(t, loader.InitialLoad(context.Background()))

	stub.config = cfg2
	require.NoError(t, loader.Reload(context.Background()))

	require.NotNil(t, callbackOld)
	require.NotNil(t, callbackNew)
	assert.Equal(t, 1, callbackOld.Version)
	assert.Equal(t, 2, callbackNew.Version)
}

func TestLoaderReload_UnchangedConfig_SkipsSwap(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig(1, "route-a")
	stub := &stubConfigLoader{config: cfg}

	callCount := 0
	loader := NewLoader(stub, WithOnReload(func(_, _ *domain.GatewayConfig) {
		callCount++
	}))

	require.NoError(t, loader.InitialLoad(context.Background()))

	// Reload with same config (same checksums).
	require.NoError(t, loader.Reload(context.Background()))

	assert.Equal(t, 0, callCount, "OnReload should not be called when config is unchanged")
}
