package wasm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cto-venkat/apiloom-ce/internal/plugin/wasm"
)

func TestCapabilitySet_AllowsHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		allowedHosts []string
		host         string
		want         bool
	}{
		{
			name:         "declared host is allowed",
			allowedHosts: []string{"api.example.com"},
			host:         "api.example.com",
			want:         true,
		},
		{
			name:         "undeclared host is blocked",
			allowedHosts: []string{"api.example.com"},
			host:         "evil.example.com",
			want:         false,
		},
		{
			name:         "empty capability set blocks all",
			allowedHosts: []string{},
			host:         "api.example.com",
			want:         false,
		},
		{
			name:         "nil capability set blocks all",
			allowedHosts: nil,
			host:         "api.example.com",
			want:         false,
		},
		{
			name:         "empty host is always blocked",
			allowedHosts: []string{"api.example.com"},
			host:         "",
			want:         false,
		},
		{
			name:         "metadata endpoint 169.254.169.254 blocked even if declared",
			allowedHosts: []string{"169.254.169.254"},
			host:         "169.254.169.254",
			want:         false,
		},
		{
			name:         "metadata endpoint metadata.google.internal blocked even if declared",
			allowedHosts: []string{"metadata.google.internal"},
			host:         "metadata.google.internal",
			want:         false,
		},
		{
			name:         "private IP 10.0.0.1 blocked even if declared",
			allowedHosts: []string{"10.0.0.1"},
			host:         "10.0.0.1",
			want:         false,
		},
		{
			name:         "private IP 192.168.1.1 blocked even if declared",
			allowedHosts: []string{"192.168.1.1"},
			host:         "192.168.1.1",
			want:         false,
		},
		{
			name:         "private IP 172.16.0.1 blocked even if declared",
			allowedHosts: []string{"172.16.0.1"},
			host:         "172.16.0.1",
			want:         false,
		},
		{
			name:         "loopback 127.0.0.1 blocked even if declared",
			allowedHosts: []string{"127.0.0.1"},
			host:         "127.0.0.1",
			want:         false,
		},
		{
			name:         "multiple allowed hosts - second one allowed",
			allowedHosts: []string{"api.example.com", "cdn.example.com"},
			host:         "cdn.example.com",
			want:         true,
		},
		{
			name:         "public IP is allowed when declared",
			allowedHosts: []string{"8.8.8.8"},
			host:         "8.8.8.8",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			caps := wasm.NewCapabilitySet(tt.allowedHosts)
			got := caps.AllowsHost(tt.host)
			assert.Equal(t, tt.want, got)
		})
	}
}
