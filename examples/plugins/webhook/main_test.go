package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/cto-venkat/apiloom-ce/api/plugin/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookOnRequest(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		received []map[string]interface{}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		mu.Lock()
		received = append(received, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	plugin := NewWebhookPlugin(WebhookConfig{
		WebhookURL: srv.URL,
		Timeout:    5 * time.Second,
	})

	req := &pluginv1.PluginRequest{
		Method: "POST",
		Path:   "/api/users",
		Headers: map[string]*pluginv1.StringList{
			"Content-Type": {Values: []string{"application/json"}},
		},
	}

	resp, err := plugin.OnRequest(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, pluginv1.Action_ACTION_CONTINUE, resp.GetAction())

	// Wait for the fire-and-forget goroutine to complete
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	payload := received[0]
	mu.Unlock()

	assert.Equal(t, "POST", payload["method"])
	assert.Equal(t, "/api/users", payload["path"])
	assert.NotEmpty(t, payload["timestamp"])
}

func TestWebhookOnResponse(t *testing.T) {
	t.Parallel()

	plugin := NewWebhookPlugin(WebhookConfig{
		WebhookURL: "http://localhost:9999",
		Timeout:    1 * time.Second,
	})

	req := &pluginv1.PluginRequest{
		Method: "GET",
		Path:   "/health",
	}

	resp, err := plugin.OnResponse(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, pluginv1.Action_ACTION_CONTINUE, resp.GetAction())
}

func TestWebhookDescribe(t *testing.T) {
	t.Parallel()

	plugin := NewWebhookPlugin(WebhookConfig{})

	resp, err := plugin.Describe(context.Background(), &pluginv1.DescribeRequest{})
	require.NoError(t, err)
	assert.Equal(t, "webhook", resp.GetName())
	assert.Equal(t, "1.0.0", resp.GetVersion())
}

func TestWebhookHealthCheck(t *testing.T) {
	t.Parallel()

	plugin := NewWebhookPlugin(WebhookConfig{})

	resp, err := plugin.HealthCheck(context.Background(), &pluginv1.HealthCheckRequest{})
	require.NoError(t, err)
	assert.True(t, resp.GetHealthy())
}

func TestWebhookFireAndForget(t *testing.T) {
	t.Parallel()

	// Webhook to a server that never responds fast enough -- request should still pass through
	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowSrv.Close()

	plugin := NewWebhookPlugin(WebhookConfig{
		WebhookURL: slowSrv.URL,
		Timeout:    100 * time.Millisecond,
	})

	req := &pluginv1.PluginRequest{
		Method: "GET",
		Path:   "/test",
	}

	resp, err := plugin.OnRequest(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, pluginv1.Action_ACTION_CONTINUE, resp.GetAction())
}
