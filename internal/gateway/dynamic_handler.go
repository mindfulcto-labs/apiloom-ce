package gateway

import (
	"log/slog"
	"net/http"
	"sync/atomic"
)

// DynamicHandler wraps a handler that gets rebuilt when the config changes.
// It uses atomic.Pointer for lock-free read access on the hot path.
type DynamicHandler struct {
	handler  atomic.Pointer[http.Handler]
	provider ConfigProvider
	chain    *BlockChain
	opts     []BuildHandlerOption
	logger   *slog.Logger
}

// NewDynamicHandler creates a handler that rebuilds on Rebuild() calls.
func NewDynamicHandler(provider ConfigProvider, chain *BlockChain, logger *slog.Logger, opts ...BuildHandlerOption) (*DynamicHandler, error) {
	dh := &DynamicHandler{
		provider: provider,
		chain:    chain,
		opts:     opts,
		logger:   logger,
	}

	// Build initial handler.
	if err := dh.Rebuild(); err != nil {
		return nil, err
	}

	return dh, nil
}

// ServeHTTP delegates to the current handler (lock-free read via atomic pointer).
func (dh *DynamicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := dh.handler.Load()
	if h != nil {
		(*h).ServeHTTP(w, r)
	} else {
		http.Error(w, "gateway not ready", http.StatusServiceUnavailable)
	}
}

// Rebuild creates a new handler from the current config and atomically swaps it in.
// In-flight requests continue with the old handler; new requests get the new one.
func (dh *DynamicHandler) Rebuild() error {
	h, err := BuildHandler(dh.provider, dh.chain, dh.opts...)
	if err != nil {
		dh.logger.Error("failed to rebuild gateway handler", "error", err)
		return err
	}

	dh.handler.Store(&h)
	dh.logger.Info("gateway handler rebuilt from config")
	return nil
}
