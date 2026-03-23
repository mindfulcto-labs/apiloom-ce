package main

import (
	"testing"

	"github.com/cto-venkat/apiloom-ce/pkg/flex"
	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseTransform(t *testing.T) {
	tests := []struct {
		name        string
		addHeaders  map[string]string
		addFields   map[string]interface{}
		backendData map[string]interface{}
		wantHeaders map[string][]string
		wantFields  map[string]interface{}
	}{
		{
			name:       "adds configured headers to response",
			addHeaders: map[string]string{"X-Transformed-By": "flex-gateway"},
			backendData: map[string]interface{}{
				"message": "hello",
			},
			wantHeaders: map[string][]string{
				"X-Transformed-By": {"flex-gateway"},
			},
			wantFields: map[string]interface{}{
				"message": "hello",
			},
		},
		{
			name: "adds configured fields to response data",
			addFields: map[string]interface{}{
				"transformed_at": "2026-01-01",
				"version":        "1.0",
			},
			backendData: map[string]interface{}{
				"data": "original",
			},
			wantFields: map[string]interface{}{
				"data":           "original",
				"transformed_at": "2026-01-01",
				"version":        "1.0",
			},
		},
		{
			name:       "empty config is no-op",
			addHeaders: map[string]string{},
			addFields:  map[string]interface{}{},
			backendData: map[string]interface{}{
				"unchanged": true,
			},
			wantFields: map[string]interface{}{
				"unchanged": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			block := NewResponseTransformBlock(ResponseTransformConfig{
				AddHeaders: tt.addHeaders,
				AddFields:  tt.addFields,
			})

			backendResp := flex.NewTestResponse(200, tt.backendData)
			tc := flex.NewTestContext().WithBackendResponse(backendResp)
			req := flex.NewTestRequest("GET", "/api/data")

			resp, err := tc.ExecuteRequest(block, req)
			require.NoError(t, err)
			require.NotNil(t, resp)

			// Check headers
			for k, v := range tt.wantHeaders {
				assert.Equal(t, v, resp.Metadata.Headers[k], "header %s", k)
			}

			// Check fields
			for k, v := range tt.wantFields {
				assert.Equal(t, v, resp.Data[k], "field %s", k)
			}
		})
	}
}

func TestResponseTransformPreservesStatusCode(t *testing.T) {
	t.Parallel()

	block := NewResponseTransformBlock(ResponseTransformConfig{
		AddHeaders: map[string]string{"X-Transformed-By": "flex"},
	})

	backendResp := &proxy.Response{
		Data:       map[string]interface{}{"ok": true},
		IsComplete: true,
		Metadata: proxy.Metadata{
			StatusCode: 201,
			Headers:    map[string][]string{"Content-Type": {"application/json"}},
		},
	}
	tc := flex.NewTestContext().WithBackendResponse(backendResp)
	req := flex.NewTestRequest("POST", "/api/create")

	resp, err := tc.ExecuteRequest(block, req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.Metadata.StatusCode)
	assert.Equal(t, []string{"application/json"}, resp.Metadata.Headers["Content-Type"])
	assert.Equal(t, []string{"flex"}, resp.Metadata.Headers["X-Transformed-By"])
}
