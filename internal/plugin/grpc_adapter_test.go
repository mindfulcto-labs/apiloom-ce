package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	pluginv1 "github.com/mindfulcto-labs/apiloom-ce/api/plugin/v1"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/plugin"
	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// mockPluginServer implements PluginServiceServer for in-process testing.
type mockPluginServer struct {
	pluginv1.UnimplementedPluginServiceServer

	onRequestFn  func(context.Context, *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error)
	onResponseFn func(context.Context, *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error)
	describeFn   func(context.Context, *pluginv1.DescribeRequest) (*pluginv1.DescribeResponse, error)
	healthFn     func(context.Context, *pluginv1.HealthCheckRequest) (*pluginv1.HealthCheckResponse, error)
}

func (m *mockPluginServer) OnRequest(ctx context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
	if m.onRequestFn != nil {
		return m.onRequestFn(ctx, req)
	}
	return &pluginv1.PluginResponse{Action: pluginv1.Action_ACTION_CONTINUE}, nil
}

func (m *mockPluginServer) OnResponse(ctx context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
	if m.onResponseFn != nil {
		return m.onResponseFn(ctx, req)
	}
	return &pluginv1.PluginResponse{Action: pluginv1.Action_ACTION_CONTINUE}, nil
}

func (m *mockPluginServer) Describe(ctx context.Context, _ *pluginv1.DescribeRequest) (*pluginv1.DescribeResponse, error) {
	if m.describeFn != nil {
		return m.describeFn(ctx, &pluginv1.DescribeRequest{})
	}
	return &pluginv1.DescribeResponse{
		Name:    "test-plugin",
		Version: "1.0.0",
	}, nil
}

func (m *mockPluginServer) HealthCheck(ctx context.Context, req *pluginv1.HealthCheckRequest) (*pluginv1.HealthCheckResponse, error) {
	if m.healthFn != nil {
		return m.healthFn(ctx, req)
	}
	return &pluginv1.HealthCheckResponse{Healthy: true, Message: "ok"}, nil
}

// startMockServer creates a gRPC server on a random port and returns the address.
func startMockServer(t *testing.T, mock *mockPluginServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	pluginv1.RegisterPluginServiceServer(srv, mock)

	go func() {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("mock gRPC server error: %v", err)
		}
	}()
	t.Cleanup(func() { srv.GracefulStop() })

	return lis.Addr().String()
}

func TestGRPCBlockName(t *testing.T) {
	t.Parallel()

	mock := &mockPluginServer{}
	addr := startMockServer(t, mock)

	block, err := plugin.NewGRPCBlock(context.Background(), plugin.GRPCBlockConfig{
		Address: addr,
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = block.Close() })

	require.Equal(t, "test-plugin", block.Name())
}

func TestGRPCBlockMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		onRequestFn    func(context.Context, *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error)
		onResponseFn   func(context.Context, *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error)
		backendResp    *proxy.Response
		wantErr        bool
		wantStatus     int
		wantRespHeader string
		wantReqHeader  string
		wantDataKey    string
		wantDataValue  string
	}{
		{
			name: "action continue passes through unchanged",
			onRequestFn: func(_ context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
				return &pluginv1.PluginResponse{Action: pluginv1.Action_ACTION_CONTINUE}, nil
			},
			onResponseFn: func(_ context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
				return &pluginv1.PluginResponse{Action: pluginv1.Action_ACTION_CONTINUE}, nil
			},
			backendResp: &proxy.Response{
				Data:       map[string]interface{}{"key": "value"},
				IsComplete: true,
				Metadata:   proxy.Metadata{StatusCode: 200, Headers: map[string][]string{"X-Backend": {"yes"}}},
			},
			wantRespHeader: "yes",
		},
		{
			name: "action modify on request adds headers",
			onRequestFn: func(_ context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
				return &pluginv1.PluginResponse{
					Action: pluginv1.Action_ACTION_MODIFY,
					Headers: map[string]*pluginv1.StringList{
						"X-Plugin-Added": {Values: []string{"injected"}},
					},
				}, nil
			},
			onResponseFn: func(_ context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
				return &pluginv1.PluginResponse{Action: pluginv1.Action_ACTION_CONTINUE}, nil
			},
			backendResp: &proxy.Response{
				Data:       map[string]interface{}{"key": "value"},
				IsComplete: true,
				Metadata:   proxy.Metadata{StatusCode: 200},
			},
			wantReqHeader: "injected",
		},
		{
			name: "action modify on response updates data",
			onRequestFn: func(_ context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
				return &pluginv1.PluginResponse{Action: pluginv1.Action_ACTION_CONTINUE}, nil
			},
			onResponseFn: func(_ context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
				return &pluginv1.PluginResponse{
					Action: pluginv1.Action_ACTION_MODIFY,
					Headers: map[string]*pluginv1.StringList{
						"X-Response-Plugin": {Values: []string{"modified"}},
					},
					ResponseData: map[string][]byte{
						"injected": []byte(`"by-plugin"`),
					},
				}, nil
			},
			backendResp: &proxy.Response{
				Data:       map[string]interface{}{"key": "value"},
				IsComplete: true,
				Metadata:   proxy.Metadata{StatusCode: 200, Headers: map[string][]string{}},
			},
			wantDataKey:   "injected",
			wantDataValue: "by-plugin",
		},
		{
			name: "action reject returns ProblemError without calling backend",
			onRequestFn: func(_ context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
				return &pluginv1.PluginResponse{
					Action:           pluginv1.Action_ACTION_REJECT,
					RejectStatusCode: 403,
					RejectType:       "forbidden",
					RejectTitle:      "Forbidden",
					RejectDetail:     "Access denied by plugin",
				}, nil
			},
			wantErr:    true,
			wantStatus: 403,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := &mockPluginServer{
				onRequestFn:  tt.onRequestFn,
				onResponseFn: tt.onResponseFn,
			}
			addr := startMockServer(t, mock)

			block, err := plugin.NewGRPCBlock(context.Background(), plugin.GRPCBlockConfig{
				Address: addr,
				Timeout: 5 * time.Second,
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = block.Close() })

			mw := block.Middleware()

			var backendCalled bool
			backendProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
				backendCalled = true
				return tt.backendResp, nil
			}

			wrapped := mw(backendProxy)

			req := &proxy.Request{
				Method:  "GET",
				Path:    "/api/test",
				Headers: map[string][]string{"Authorization": {"Bearer token123"}},
			}

			resp, err := wrapped(context.Background(), req)

			if tt.wantErr {
				require.Error(t, err)
				var pe *domain.ProblemError
				require.ErrorAs(t, err, &pe)
				require.Equal(t, tt.wantStatus, pe.Status)
				require.False(t, backendCalled, "backend should not be called on reject")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)

			if tt.wantRespHeader != "" {
				require.Equal(t, tt.wantRespHeader, resp.Metadata.Headers["X-Backend"][0])
			}
			if tt.wantReqHeader != "" {
				// The request should have been modified before backend call
				require.Equal(t, tt.wantReqHeader, req.Headers["X-Plugin-Added"][0])
			}
			if tt.wantDataKey != "" {
				val, ok := resp.Data[tt.wantDataKey]
				require.True(t, ok, "expected data key %q", tt.wantDataKey)
				// Response data comes back as json.RawMessage decoded
				require.Equal(t, tt.wantDataValue, val)
			}
		})
	}
}

func TestGRPCBlockBodyPassthrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		bodyPassthrough bool
		wantBody        bool
	}{
		{name: "body passthrough enabled", bodyPassthrough: true, wantBody: true},
		{name: "body passthrough disabled", bodyPassthrough: false, wantBody: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var receivedBody []byte
			mock := &mockPluginServer{
				onRequestFn: func(_ context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
					receivedBody = req.Body
					return &pluginv1.PluginResponse{Action: pluginv1.Action_ACTION_CONTINUE}, nil
				},
			}
			addr := startMockServer(t, mock)

			block, err := plugin.NewGRPCBlock(context.Background(), plugin.GRPCBlockConfig{
				Address:         addr,
				Timeout:         5 * time.Second,
				BodyPassthrough: tt.bodyPassthrough,
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = block.Close() })

			mw := block.Middleware()
			backendProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
				return &proxy.Response{
					Data:       map[string]interface{}{},
					IsComplete: true,
					Metadata:   proxy.Metadata{StatusCode: 200},
				}, nil
			}

			wrapped := mw(backendProxy)

			req := &proxy.Request{
				Method:  "POST",
				Path:    "/api/data",
				Headers: map[string][]string{},
				Body:    io.NopCloser(strings.NewReader(`{"test":"data"}`)),
			}

			_, err = wrapped(context.Background(), req)
			require.NoError(t, err)

			if tt.wantBody {
				require.NotEmpty(t, receivedBody)
			} else {
				require.Empty(t, receivedBody)
			}
		})
	}
}

func TestGRPCBlockTimeout(t *testing.T) {
	t.Parallel()

	mock := &mockPluginServer{
		onRequestFn: func(ctx context.Context, _ *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
			// Block until context is canceled
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	addr := startMockServer(t, mock)

	block, err := plugin.NewGRPCBlock(context.Background(), plugin.GRPCBlockConfig{
		Address: addr,
		Timeout: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = block.Close() })

	mw := block.Middleware()
	backendProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{Data: map[string]interface{}{}, Metadata: proxy.Metadata{StatusCode: 200}}, nil
	}

	wrapped := mw(backendProxy)
	req := &proxy.Request{Method: "GET", Path: "/timeout", Headers: map[string][]string{}}

	_, err = wrapped(context.Background(), req)
	require.Error(t, err)

	var pe *domain.ProblemError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, 502, pe.Status)
}

func TestGRPCBlockFailOpen(t *testing.T) {
	t.Parallel()

	// Use an address that will fail to connect on RPC calls
	// We need a real server for Describe to succeed, then shut it down
	mock := &mockPluginServer{}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()

	srv := grpc.NewServer()
	pluginv1.RegisterPluginServiceServer(srv, mock)
	go func() { _ = srv.Serve(lis) }()

	block, err := plugin.NewGRPCBlock(context.Background(), plugin.GRPCBlockConfig{
		Address:  addr,
		Timeout:  200 * time.Millisecond,
		FailOpen: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = block.Close() })

	// Now stop the server to simulate unreachable plugin
	srv.GracefulStop()

	mw := block.Middleware()
	backendCalled := false
	backendProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		backendCalled = true
		return &proxy.Response{
			Data:       map[string]interface{}{"ok": true},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: 200},
		}, nil
	}

	wrapped := mw(backendProxy)
	req := &proxy.Request{Method: "GET", Path: "/failopen", Headers: map[string][]string{}}

	resp, err := wrapped(context.Background(), req)
	require.NoError(t, err, "fail-open should not return error")
	require.True(t, backendCalled, "backend should still be called in fail-open mode")
	require.NotNil(t, resp)
}

func TestGRPCBlockDescriptor(t *testing.T) {
	t.Parallel()

	schema := json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}}}`)
	mock := &mockPluginServer{
		describeFn: func(_ context.Context, _ *pluginv1.DescribeRequest) (*pluginv1.DescribeResponse, error) {
			return &pluginv1.DescribeResponse{
				Name:         "custom-auth",
				Version:      "2.1.0",
				ConfigSchema: schema,
			}, nil
		},
	}
	addr := startMockServer(t, mock)

	block, err := plugin.NewGRPCBlock(context.Background(), plugin.GRPCBlockConfig{
		Address: addr,
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = block.Close() })

	require.Equal(t, "custom-auth", block.Name())
	desc := block.Descriptor()
	require.Equal(t, "custom-auth", desc.Name)
	require.Equal(t, "2.1.0", desc.Version)
	require.JSONEq(t, string(schema), string(desc.ConfigSchema))
}

func TestNewGRPCBlockDialOptions(t *testing.T) {
	t.Parallel()

	mock := &mockPluginServer{}
	addr := startMockServer(t, mock)

	// Test with custom dial options
	block, err := plugin.NewGRPCBlock(context.Background(), plugin.GRPCBlockConfig{
		Address: addr,
		Timeout: 5 * time.Second,
	}, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = block.Close() })

	require.Equal(t, "test-plugin", block.Name())
}
