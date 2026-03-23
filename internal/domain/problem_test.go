package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProblemError(t *testing.T) {
	t.Parallel()

	t.Run("Error returns human-readable string", func(t *testing.T) {
		t.Parallel()
		pe := NewProblemError(401, "unauthorized", "invalid API key")
		assert.Equal(t, "Unauthorized: invalid API key", pe.Error())
	})

	t.Run("StatusCode returns HTTP status", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			status int
		}{
			{name: "401 unauthorized", status: 401},
			{name: "403 forbidden", status: 403},
			{name: "413 payload too large", status: 413},
			{name: "429 rate limited", status: 429},
			{name: "500 internal error", status: 500},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				pe := NewProblemError(tt.status, "test", "detail")
				assert.Equal(t, tt.status, pe.StatusCode())
			})
		}
	})

	t.Run("MarshalJSON produces RFC 7807 JSON", func(t *testing.T) {
		t.Parallel()
		pe := NewProblemError(401, "unauthorized", "invalid API key")
		data, err := json.Marshal(pe)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		assert.Equal(t, "urn:flexgateway:error:unauthorized", result["type"])
		assert.Equal(t, "Unauthorized", result["title"])
		assert.Equal(t, float64(401), result["status"])
		assert.Equal(t, "invalid API key", result["detail"])
	})

	t.Run("MarshalJSON titles from status codes", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			status int
			title  string
		}{
			{status: 400, title: "Bad Request"},
			{status: 401, title: "Unauthorized"},
			{status: 403, title: "Forbidden"},
			{status: 404, title: "Not Found"},
			{status: 413, title: "Request Entity Too Large"},
			{status: 429, title: "Too Many Requests"},
			{status: 500, title: "Internal Server Error"},
		}
		for _, tt := range tests {
			t.Run(tt.title, func(t *testing.T) {
				t.Parallel()
				pe := NewProblemError(tt.status, "test", "detail")
				data, err := json.Marshal(pe)
				require.NoError(t, err)

				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				assert.Equal(t, tt.title, result["title"])
			})
		}
	})

	t.Run("NewProblemError constructs correct URN type", func(t *testing.T) {
		t.Parallel()
		pe := NewProblemError(403, "rate-exceeded", "too many requests")
		assert.Equal(t, "urn:flexgateway:error:rate-exceeded", pe.Type)
	})
}
