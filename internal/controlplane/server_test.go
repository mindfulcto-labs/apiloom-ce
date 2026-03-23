package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testOrgHeader = "00000000-0000-0000-0000-000000000001"

func addOrgHeader(req *http.Request) *http.Request {
	req.Header.Set("X-Organization-ID", testOrgHeader)
	return req
}

func TestServerHandler_StatusEndpoint(t *testing.T) {
	srv := NewServer(nil, "test-version", time.Now())
	handler := srv.Handler()

	req := addOrgHeader(httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestServerHandler_RoutesEndpointMounted(t *testing.T) {
	srv := NewServer(nil, "test-version", time.Now())
	handler := srv.Handler()

	// GET /api/v1/routes should reach RouteHandler (405 would mean not mounted)
	req := addOrgHeader(httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Without a real store, this will hit a nil pointer or return an error,
	// but the status code tells us the route is mounted.
	// A 404 means the route is NOT mounted; anything else means it is.
	if rec.Code == http.StatusNotFound {
		t.Fatal("expected /api/v1/routes to be mounted, got 404")
	}
}

func TestServerHandler_BackendsEndpointMounted(t *testing.T) {
	srv := NewServer(nil, "test-version", time.Now())
	handler := srv.Handler()

	req := addOrgHeader(httptest.NewRequest(http.MethodGet, "/api/v1/backends", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatal("expected /api/v1/backends to be mounted, got 404")
	}
}

func TestServerHandler_ConsumersEndpointMounted(t *testing.T) {
	srv := NewServer(nil, "test-version", time.Now())
	handler := srv.Handler()

	req := addOrgHeader(httptest.NewRequest(http.MethodGet, "/api/v1/consumers", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatal("expected /api/v1/consumers to be mounted, got 404")
	}
}

func TestServerHandler_AIModelsEndpointMounted(t *testing.T) {
	srv := NewServer(nil, "test-version", time.Now())
	handler := srv.Handler()

	req := addOrgHeader(httptest.NewRequest(http.MethodGet, "/api/v1/ai/models", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatal("expected /api/v1/ai/models to be mounted, got 404")
	}
}

func TestServerHandler_AIBudgetsEndpointMounted(t *testing.T) {
	srv := NewServer(nil, "test-version", time.Now())
	handler := srv.Handler()

	req := addOrgHeader(httptest.NewRequest(http.MethodGet, "/api/v1/ai/budgets", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatal("expected /api/v1/ai/budgets to be mounted, got 404")
	}
}

func TestServerHandler_UnknownPathReturns404(t *testing.T) {
	srv := NewServer(nil, "test-version", time.Now())
	handler := srv.Handler()

	req := addOrgHeader(httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 404 or 405 for unknown path, got %d", rec.Code)
	}
}

func TestServerHandler_MissingOrgHeaderReturns400(t *testing.T) {
	srv := NewServer(nil, "test-version", time.Now())
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing org header, got %d", rec.Code)
	}
}
