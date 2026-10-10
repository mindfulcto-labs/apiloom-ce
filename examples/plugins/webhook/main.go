// Package main demonstrates a gRPC plugin that sends webhook notifications
// on incoming requests. The webhook POST is fire-and-forget -- the request
// continues to the backend regardless of whether the webhook call succeeds.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	pluginv1 "github.com/mindfulcto-labs/apiloom-ce/api/plugin/v1"
	"google.golang.org/grpc"
)

// WebhookConfig holds the configuration for the webhook plugin.
type WebhookConfig struct {
	// WebhookURL is the target URL for POST notifications.
	WebhookURL string
	// Timeout for the HTTP POST to the webhook URL (default 5s).
	Timeout time.Duration
}

// WebhookPlugin implements the gRPC PluginServiceServer and sends webhook
// notifications on request events.
type WebhookPlugin struct {
	pluginv1.UnimplementedPluginServiceServer
	config     WebhookConfig
	httpClient *http.Client
}

// NewWebhookPlugin creates a WebhookPlugin with the given configuration.
func NewWebhookPlugin(cfg WebhookConfig) *WebhookPlugin {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &WebhookPlugin{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// webhookPayload is the JSON body sent to the webhook URL.
type webhookPayload struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Timestamp string            `json:"timestamp"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// OnRequest fires a goroutine to POST the request details to the webhook URL
// and immediately returns ACTION_CONTINUE.
func (p *WebhookPlugin) OnRequest(ctx context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
	payload := webhookPayload{
		Method:    req.GetMethod(),
		Path:      req.GetPath(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Headers:   flattenHeaders(req.GetHeaders()),
	}

	// Fire and forget: the webhook keeps the request's values (for tracing)
	// but is not cancelled when the request finishes.
	go p.fireWebhook(context.WithoutCancel(ctx), payload)

	return &pluginv1.PluginResponse{
		Action: pluginv1.Action_ACTION_CONTINUE,
	}, nil
}

// OnResponse is a no-op for the webhook plugin.
func (p *WebhookPlugin) OnResponse(_ context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
	return &pluginv1.PluginResponse{
		Action: pluginv1.Action_ACTION_CONTINUE,
	}, nil
}

// Describe returns metadata about this plugin.
func (p *WebhookPlugin) Describe(_ context.Context, _ *pluginv1.DescribeRequest) (*pluginv1.DescribeResponse, error) {
	return &pluginv1.DescribeResponse{
		Name:    "webhook",
		Version: "1.0.0",
	}, nil
}

// HealthCheck returns healthy=true.
func (p *WebhookPlugin) HealthCheck(_ context.Context, _ *pluginv1.HealthCheckRequest) (*pluginv1.HealthCheckResponse, error) {
	return &pluginv1.HealthCheckResponse{
		Healthy: true,
		Message: "webhook plugin operational",
	}, nil
}

// fireWebhook sends the payload to the configured webhook URL.
func (p *WebhookPlugin) fireWebhook(parent context.Context, payload webhookPayload) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("webhook: failed to marshal payload: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(parent, p.config.Timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: failed to create request: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("webhook: POST failed: %v", err)
		return
	}
	_ = resp.Body.Close()
}

// flattenHeaders converts protobuf StringList headers to single-value map.
func flattenHeaders(headers map[string]*pluginv1.StringList) map[string]string {
	if headers == nil {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if len(v.GetValues()) > 0 {
			out[k] = v.GetValues()[0]
		}
	}
	return out
}

func main() {
	port := 50051
	if v := os.Getenv("PLUGIN_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			log.Fatal("webhook plugin: PLUGIN_PORT must be a number from 1 to 65535")
		}
		port = n
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("webhook plugin: failed to listen on port %d: %v", port, err)
	}

	srv := grpc.NewServer()
	plugin := NewWebhookPlugin(WebhookConfig{
		WebhookURL: os.Getenv("WEBHOOK_URL"),
		Timeout:    5 * time.Second,
	})
	pluginv1.RegisterPluginServiceServer(srv, plugin)

	log.Printf("webhook plugin: listening on :%d", port)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("webhook plugin: server failed: %v", err)
	}
}
