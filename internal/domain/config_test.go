package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeChecksum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   json.RawMessage
		wantSame bool
		other    json.RawMessage
	}{
		{
			name:     "same input produces same hash",
			config:   json.RawMessage(`{"host":"example.com"}`),
			wantSame: true,
			other:    json.RawMessage(`{"host":"example.com"}`),
		},
		{
			name:     "different input produces different hash",
			config:   json.RawMessage(`{"host":"example.com"}`),
			wantSame: false,
			other:    json.RawMessage(`{"host":"other.com"}`),
		},
		{
			name:     "empty JSON object",
			config:   json.RawMessage(`{}`),
			wantSame: true,
			other:    json.RawMessage(`{}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sum1 := ComputeChecksum(tt.config)
			sum2 := ComputeChecksum(tt.other)

			require.NotEmpty(t, sum1)
			require.Len(t, sum1, 64, "SHA-256 hex should be 64 characters")

			if tt.wantSame {
				assert.Equal(t, sum1, sum2)
			} else {
				assert.NotEqual(t, sum1, sum2)
			}
		})
	}
}

func TestGatewayConfigStructure(t *testing.T) {
	t.Parallel()

	// Verify GatewayConfig can be marshaled/unmarshaled with all fields.
	cfg := GatewayConfig{
		Version: 1,
		Routes: []RouteConfig{
			{Name: "test-route", Config: json.RawMessage(`{"path":"/api"}`)},
		},
		Backends: []BackendConfig{
			{Name: "test-backend", Config: json.RawMessage(`{"url":"http://localhost"}`)},
		},
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	var decoded GatewayConfig
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, 1, decoded.Version)
	assert.Len(t, decoded.Routes, 1)
	assert.Len(t, decoded.Backends, 1)
	assert.Equal(t, "test-route", decoded.Routes[0].Name)
	assert.Equal(t, "test-backend", decoded.Backends[0].Name)
}
