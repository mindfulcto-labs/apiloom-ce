//go:build integration

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/testutil"
)

func setupIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	return pool
}

func TestHealthIntegrationReadyz(t *testing.T) {
	pool := setupIntegrationPool(t)

	h := NewHealthHandler(
		&mockConfigProvider{config: &domain.GatewayConfig{Version: 1}},
		pool,
	)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Status)
}

func TestHealthIntegrationDeep(t *testing.T) {
	pool := setupIntegrationPool(t)

	h := NewHealthHandler(
		&mockConfigProvider{config: &domain.GatewayConfig{Version: 1}},
		pool,
	)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/healthz/deep", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Real Postgres latency should be present
	pgCheck, ok := resp.Checks["postgres"]
	require.True(t, ok)
	assert.Equal(t, "ok", pgCheck.Status)
	assert.NotEmpty(t, pgCheck.Latency)
}
