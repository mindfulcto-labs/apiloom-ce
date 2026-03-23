package gateway

import (
	"context"
	"testing"
)

// residencyCtxKey is a typed context key for data residency region.
// Mirrors the production key in residency.go.

func TestRouteByResidency(t *testing.T) {
	t.Parallel()

	backends := ResidencyConfig{
		Regions: map[string]string{
			"EU":   "https://eu.api.example.com",
			"US":   "https://us.api.example.com",
			"APAC": "https://apac.api.example.com",
		},
		Default: "https://api.example.com",
	}

	router := NewResidencyRouter(nil) // logger nil uses slog.Default

	tests := []struct {
		name     string
		region   string
		wantURL  string
		wantName string
	}{
		{
			name:     "EU consumer routed to EU backend",
			region:   "EU",
			wantURL:  "https://eu.api.example.com",
			wantName: "EU",
		},
		{
			name:     "US consumer routed to US backend",
			region:   "US",
			wantURL:  "https://us.api.example.com",
			wantName: "US",
		},
		{
			name:     "APAC consumer routed to APAC backend",
			region:   "APAC",
			wantURL:  "https://apac.api.example.com",
			wantName: "APAC",
		},
		{
			name:     "unknown region falls back to default",
			region:   "LATAM",
			wantURL:  "https://api.example.com",
			wantName: "default",
		},
		{
			name:     "empty region falls back to default",
			region:   "",
			wantURL:  "https://api.example.com",
			wantName: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			if tt.region != "" {
				ctx = WithResidencyRegion(ctx, tt.region)
			}

			backend, err := router.RouteByResidency(ctx, backends)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if backend.URL != tt.wantURL {
				t.Errorf("got URL %q, want %q", backend.URL, tt.wantURL)
			}
			if backend.Region != tt.wantName {
				t.Errorf("got Region %q, want %q", backend.Region, tt.wantName)
			}
		})
	}
}

func TestRouteByResidency_NoBackends(t *testing.T) {
	t.Parallel()

	router := NewResidencyRouter(nil)
	cfg := ResidencyConfig{
		Regions: map[string]string{},
		Default: "",
	}

	ctx := WithResidencyRegion(context.Background(), "EU")
	_, err := router.RouteByResidency(ctx, cfg)
	if err == nil {
		t.Fatal("expected error for empty config, got nil")
	}
}

func TestResidencyRegionContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if got := ResidencyRegionFromCtx(ctx); got != "" {
		t.Errorf("expected empty region from bare context, got %q", got)
	}

	ctx = WithResidencyRegion(ctx, "EU")
	if got := ResidencyRegionFromCtx(ctx); got != "EU" {
		t.Errorf("expected EU, got %q", got)
	}
}
