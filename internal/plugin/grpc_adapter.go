// Package plugin provides the gRPC adapter for external plugins.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	pluginv1 "github.com/cto-venkat/apiloom-ce/api/plugin/v1"
	"github.com/cto-venkat/apiloom-ce/internal/domain"
	"github.com/cto-venkat/apiloom-ce/pkg/flex"
	"github.com/luraproject/lura/v2/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCBlockConfig configures a GRPCBlock connection to an external plugin.
type GRPCBlockConfig struct {
	// Address is the gRPC target (host:port).
	Address string

	// Timeout for individual gRPC calls (default 5s).
	Timeout time.Duration

	// BodyPassthrough sends the request body over gRPC when true.
	BodyPassthrough bool

	// FailOpen passes requests through to backend when the plugin is unreachable.
	FailOpen bool
}

// GRPCBlock adapts an external gRPC plugin process into the Block interface.
// It calls OnRequest before the backend and OnResponse after.
type GRPCBlock struct {
	client     pluginv1.PluginServiceClient
	conn       *grpc.ClientConn
	name       string
	descriptor flex.PluginDescriptor
	timeout    time.Duration
	bodyPass   bool
	failOpen   bool
}

// NewGRPCBlock dials the external plugin, calls Describe to get metadata,
// and returns a Block-compatible adapter. Extra dial options can be appended.
func NewGRPCBlock(ctx context.Context, cfg GRPCBlockConfig, extraOpts ...grpc.DialOption) (*GRPCBlock, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	opts = append(opts, extraOpts...)

	conn, err := grpc.NewClient(cfg.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing plugin at %s: %w", cfg.Address, err)
	}

	client := pluginv1.NewPluginServiceClient(conn)

	descCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	desc, err := client.Describe(descCtx, &pluginv1.DescribeRequest{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("describing plugin at %s: %w", cfg.Address, err)
	}

	return &GRPCBlock{
		client: client,
		conn:   conn,
		name:   desc.GetName(),
		descriptor: flex.PluginDescriptor{
			Name:         desc.GetName(),
			Version:      desc.GetVersion(),
			Type:         flex.PluginTypeGRPC,
			ConfigSchema: desc.GetConfigSchema(),
		},
		timeout:  cfg.Timeout,
		bodyPass: cfg.BodyPassthrough,
		failOpen: cfg.FailOpen,
	}, nil
}

// Name returns the plugin name obtained from the Describe RPC.
func (g *GRPCBlock) Name() string { return g.name }

// Descriptor returns the full plugin descriptor including version and config schema.
func (g *GRPCBlock) Descriptor() flex.PluginDescriptor { return g.descriptor }

// Middleware returns a Lura proxy.Middleware that calls the external plugin
// for request and response processing.
func (g *GRPCBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) == 0 {
			// Should never happen in a real chain, but be safe.
			return proxy.NoopProxy
		}
		backend := next[0]

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			// --- OnRequest phase ---
			resp, err := g.callOnRequest(ctx, req)
			if err != nil {
				return resp, err
			}

			// --- Call backend ---
			backendResp, err := backend(ctx, req)
			if err != nil {
				return nil, err
			}

			// --- OnResponse phase ---
			return g.callOnResponse(ctx, req, backendResp)
		}
	}
}

// callOnRequest converts the proxy request to proto, calls OnRequest,
// and applies the plugin's decision.
func (g *GRPCBlock) callOnRequest(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
	protoReq, err := proxyReqToProto(req, g.bodyPass)
	if err != nil {
		return nil, fmt.Errorf("converting request to proto: %w", err)
	}

	rpcCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	protoResp, err := g.client.OnRequest(rpcCtx, protoReq)
	if err != nil {
		if g.failOpen {
			return nil, nil //nolint:nilnil // nil,nil means continue to backend
		}
		return nil, domain.NewProblemError(502, "plugin-unreachable",
			fmt.Sprintf("plugin %q OnRequest failed: %v", g.name, err))
	}

	switch protoResp.GetAction() {
	case pluginv1.Action_ACTION_REJECT:
		return nil, domain.NewProblemError(
			int(protoResp.GetRejectStatusCode()),
			protoResp.GetRejectType(),
			protoResp.GetRejectDetail(),
		)
	case pluginv1.Action_ACTION_MODIFY:
		applyRequestModifications(req, protoResp)
	}

	return nil, nil //nolint:nilnil // nil,nil means continue to backend
}

// callOnResponse converts the proxy response to proto, calls OnResponse,
// and applies modifications.
func (g *GRPCBlock) callOnResponse(ctx context.Context, req *proxy.Request, resp *proxy.Response) (*proxy.Response, error) {
	protoReq := proxyRespToProto(req, resp)

	rpcCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	protoResp, err := g.client.OnResponse(rpcCtx, protoReq)
	if err != nil {
		if g.failOpen {
			return resp, nil
		}
		return nil, domain.NewProblemError(502, "plugin-unreachable",
			fmt.Sprintf("plugin %q OnResponse failed: %v", g.name, err))
	}

	if protoResp.GetAction() == pluginv1.Action_ACTION_MODIFY {
		applyResponseModifications(resp, protoResp)
	}

	return resp, nil
}

// Close closes the underlying gRPC connection.
func (g *GRPCBlock) Close() error {
	return g.conn.Close()
}

// proxyReqToProto converts a Lura proxy.Request into a PluginRequest protobuf.
func proxyReqToProto(req *proxy.Request, bodyPassthrough bool) (*pluginv1.PluginRequest, error) {
	proto := &pluginv1.PluginRequest{
		Method:  req.Method,
		Path:    req.Path,
		Headers: headersToProto(req.Headers),
	}

	if bodyPassthrough && req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
		proto.Body = body
	}

	return proto, nil
}

// applyRequestModifications applies header changes from the plugin response
// onto the proxy request.
func applyRequestModifications(req *proxy.Request, resp *pluginv1.PluginResponse) {
	if req.Headers == nil {
		req.Headers = make(map[string][]string)
	}
	for k, v := range resp.GetHeaders() {
		req.Headers[k] = v.GetValues()
	}
}

// proxyRespToProto converts a proxy request+response pair into a PluginRequest
// for the OnResponse RPC.
func proxyRespToProto(req *proxy.Request, resp *proxy.Response) *pluginv1.PluginRequest {
	proto := &pluginv1.PluginRequest{
		Method:     req.Method,
		Path:       req.Path,
		Headers:    headersToProto(resp.Metadata.Headers),
		StatusCode: int32(resp.Metadata.StatusCode),
	}

	if resp.Data != nil {
		proto.ResponseData = make(map[string][]byte, len(resp.Data))
		for k, v := range resp.Data {
			b, err := json.Marshal(v)
			if err != nil {
				continue // skip unserializable data
			}
			proto.ResponseData[k] = b
		}
	}

	return proto
}

// applyResponseModifications applies header and data changes from the plugin
// response onto the proxy response.
func applyResponseModifications(resp *proxy.Response, protoResp *pluginv1.PluginResponse) {
	if resp.Metadata.Headers == nil {
		resp.Metadata.Headers = make(map[string][]string)
	}
	for k, v := range protoResp.GetHeaders() {
		resp.Metadata.Headers[k] = v.GetValues()
	}

	if resp.Data == nil {
		resp.Data = make(map[string]interface{})
	}
	for k, v := range protoResp.GetResponseData() {
		var decoded interface{}
		if err := json.Unmarshal(v, &decoded); err != nil {
			resp.Data[k] = string(v)
			continue
		}
		resp.Data[k] = decoded
	}
}

// headersToProto converts Go HTTP headers to the protobuf StringList map.
func headersToProto(headers map[string][]string) map[string]*pluginv1.StringList {
	if headers == nil {
		return nil
	}
	out := make(map[string]*pluginv1.StringList, len(headers))
	for k, v := range headers {
		out[k] = &pluginv1.StringList{Values: v}
	}
	return out
}
