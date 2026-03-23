package main

import (
	"encoding/base64"
	"testing"

	"github.com/cto-venkat/apiloom-ce/pkg/flex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomAuth(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		header     string
		prefix     string
		wantStatus int
		wantErr    bool
	}{
		{
			name:       "valid token in X-Custom-Token passes",
			token:      "flex-" + base64.StdEncoding.EncodeToString([]byte("user:secret")),
			header:     "X-Custom-Token",
			wantStatus: 0,
			wantErr:    false,
		},
		{
			name:       "valid token in Authorization passes",
			token:      "flex-" + base64.StdEncoding.EncodeToString([]byte("admin:pass")),
			header:     "Authorization",
			wantStatus: 0,
			wantErr:    false,
		},
		{
			name:       "missing token rejected with 401",
			token:      "",
			header:     "",
			wantStatus: 401,
			wantErr:    true,
		},
		{
			name:       "wrong prefix rejected with 401",
			token:      "bearer-" + base64.StdEncoding.EncodeToString([]byte("user:secret")),
			header:     "X-Custom-Token",
			wantStatus: 401,
			wantErr:    true,
		},
		{
			name:       "invalid base64 after prefix rejected",
			token:      "flex-!!!notbase64!!!",
			header:     "X-Custom-Token",
			wantStatus: 401,
			wantErr:    true,
		},
		{
			name:       "custom prefix works",
			token:      "myapp-" + base64.StdEncoding.EncodeToString([]byte("data")),
			header:     "X-Custom-Token",
			prefix:     "myapp-",
			wantStatus: 0,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prefix := "flex-"
			if tt.prefix != "" {
				prefix = tt.prefix
			}
			block := NewCustomAuthBlock(CustomAuthConfig{
				TokenPrefix:   prefix,
				RequireBase64: true,
			})

			tc := flex.NewTestContext()
			req := flex.NewTestRequest("GET", "/api/resource")
			if tt.header != "" && tt.token != "" {
				req.Headers[tt.header] = []string{tt.token}
			}

			resp, err := tc.ExecuteRequest(block, req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
			}
		})
	}
}
