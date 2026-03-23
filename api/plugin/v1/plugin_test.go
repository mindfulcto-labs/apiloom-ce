package pluginv1_test

import (
	"testing"

	pluginv1 "github.com/cto-venkat/apiloom-ce/api/plugin/v1"
	"github.com/stretchr/testify/require"
)

func TestProtoTypesExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "PluginRequest has all fields",
			fn: func() {
				req := &pluginv1.PluginRequest{
					Method:   "GET",
					Path:     "/api/test",
					Headers:  map[string]*pluginv1.StringList{"Accept": {Values: []string{"application/json"}}},
					Body:     []byte("body"),
					Metadata: map[string]string{"consumer_id": "c1"},
				}
				require.Equal(t, "GET", req.GetMethod())
				require.Equal(t, "/api/test", req.GetPath())
				require.Equal(t, []string{"application/json"}, req.GetHeaders()["Accept"].GetValues())
			},
		},
		{
			name: "PluginResponse action enum values",
			fn: func() {
				require.Equal(t, int32(0), int32(pluginv1.Action_ACTION_UNSPECIFIED))
				require.Equal(t, int32(1), int32(pluginv1.Action_ACTION_CONTINUE))
				require.Equal(t, int32(2), int32(pluginv1.Action_ACTION_MODIFY))
				require.Equal(t, int32(3), int32(pluginv1.Action_ACTION_REJECT))
			},
		},
		{
			name: "PluginResponse reject fields",
			fn: func() {
				resp := &pluginv1.PluginResponse{
					Action:           pluginv1.Action_ACTION_REJECT,
					RejectStatusCode: 403,
					RejectType:       "forbidden",
					RejectTitle:      "Forbidden",
					RejectDetail:     "Access denied",
				}
				require.Equal(t, pluginv1.Action_ACTION_REJECT, resp.GetAction())
				require.Equal(t, int32(403), resp.GetRejectStatusCode())
			},
		},
		{
			name: "DescribeResponse fields",
			fn: func() {
				desc := &pluginv1.DescribeResponse{
					Name:         "test-plugin",
					Version:      "1.0.0",
					ConfigSchema: []byte(`{"type":"object"}`),
				}
				require.Equal(t, "test-plugin", desc.GetName())
				require.Equal(t, "1.0.0", desc.GetVersion())
			},
		},
		{
			name: "HealthCheckResponse fields",
			fn: func() {
				hc := &pluginv1.HealthCheckResponse{
					Healthy: true,
					Message: "ok",
				}
				require.True(t, hc.GetHealthy())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.fn()
		})
	}
}
