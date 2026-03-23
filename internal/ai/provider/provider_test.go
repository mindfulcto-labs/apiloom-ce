package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProviderURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid public URL", url: "https://api.openai.com/v1", wantErr: false},
		{name: "valid public URL with port", url: "https://api.anthropic.com:443/v1", wantErr: false},
		{name: "private 10.x.x.x", url: "http://10.0.0.1/v1", wantErr: true},
		{name: "private 172.16.x.x", url: "http://172.16.0.1/v1", wantErr: true},
		{name: "private 192.168.x.x", url: "http://192.168.1.1/v1", wantErr: true},
		{name: "link-local 169.254.x.x", url: "http://169.254.1.1/v1", wantErr: true},
		{name: "cloud metadata endpoint", url: "http://169.254.169.254/latest/meta-data/", wantErr: true},
		{name: "localhost", url: "http://localhost/v1", wantErr: true},
		{name: "loopback 127.0.0.1", url: "http://127.0.0.1/v1", wantErr: true},
		{name: "IPv6 loopback", url: "http://[::1]/v1", wantErr: true},
		{name: "IPv6 private fd00::", url: "http://[fd00::1]/v1", wantErr: true},
		{name: "empty URL", url: "", wantErr: true},
		{name: "invalid URL", url: "not-a-url", wantErr: true},
		{name: "no scheme", url: "api.openai.com/v1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateProviderURL(tt.url)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "provider URL")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
