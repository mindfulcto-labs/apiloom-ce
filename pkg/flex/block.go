// Package flex is the public plugin SDK for FlexGateway.
// It is the ONLY exported package. External developers import this package
// to build plugins using the Block interface.
package flex

import (
	"encoding/json"

	"github.com/luraproject/lura/v2/proxy"
)

// Block is the extension point for all middleware in the gateway pipeline.
// Every plugin implements Block. This mirrors the internal gateway.Block
// interface but lives in the public package so external developers have
// no dependency on internal packages.
type Block interface {
	// Name returns a unique identifier for the block (e.g., "auth-jwt", "rate-limit").
	Name() string

	// Middleware returns a Lura-compatible proxy middleware that wraps the next proxy
	// in the chain.
	Middleware() proxy.Middleware
}

// PluginType identifies the execution model for a plugin.
type PluginType int

const (
	// PluginTypeNative is a plugin compiled into the gateway binary.
	PluginTypeNative PluginType = iota
	// PluginTypeGRPC is a plugin running as a separate gRPC process.
	PluginTypeGRPC
)

// String returns the string representation of a PluginType.
func (pt PluginType) String() string {
	switch pt {
	case PluginTypeNative:
		return "native"
	case PluginTypeGRPC:
		return "grpc"
	default:
		return "unknown"
	}
}

// PluginDescriptor holds metadata about a plugin for registration and discovery.
type PluginDescriptor struct {
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Type         PluginType      `json:"type"`
	ConfigSchema json.RawMessage `json:"config_schema,omitempty"`
}
