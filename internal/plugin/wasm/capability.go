// Package wasm provides a sandboxed WASM plugin execution environment using Wazero.
// Plugins run with no host filesystem access, no host memory access, and only
// declared network targets accessible via the http_fetch host function.
package wasm

import (
	"net"
)

// privateNetCIDRs are always blocked regardless of declared hosts (SSRF prevention).
var privateNetCIDRs = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"127.0.0.0/8",
	"fc00::/7",
}

// metadataHosts are cloud metadata endpoints — always blocked.
var metadataHosts = []string{
	"169.254.169.254",
	"metadata.google.internal",
}

// defaultPrivateNets returns parsed private network CIDRs.
// Called once at package load via var initialization (not init()).
func defaultPrivateNets() []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(privateNetCIDRs))
	for _, cidr := range privateNetCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

// blockedNets is the package-level list of blocked private network ranges.
// Initialized via var assignment (avoids init() function).
var blockedNets = defaultPrivateNets() //nolint:gochecknoglobals

// CapabilitySet declares the network hosts a WASM plugin is allowed to reach.
// An empty CapabilitySet denies all outbound connections.
// Private IP ranges and cloud metadata endpoints are always denied regardless
// of what is declared in AllowedHosts.
type CapabilitySet struct {
	allowedHosts map[string]struct{}
}

// NewCapabilitySet constructs a CapabilitySet from a slice of allowed hostnames
// or IP addresses. Metadata endpoints and private IP ranges are always blocked.
func NewCapabilitySet(hosts []string) CapabilitySet {
	m := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		if h != "" {
			m[h] = struct{}{}
		}
	}
	return CapabilitySet{allowedHosts: m}
}

// AllowsHost returns true if the given host is explicitly declared and not
// in a blocked range (private IP, metadata endpoint).
//
// Security invariant: private ranges and metadata endpoints are ALWAYS denied,
// even if explicitly declared in AllowedHosts. Fail-closed.
func (cs CapabilitySet) AllowsHost(host string) bool {
	if host == "" {
		return false
	}

	// Check metadata hosts (by name — these may not be IPs).
	for _, mh := range metadataHosts {
		if host == mh {
			return false
		}
	}

	// Check private IP ranges.
	ip := net.ParseIP(host)
	if ip != nil {
		for _, pn := range blockedNets {
			if pn.Contains(ip) {
				return false
			}
		}
	}

	// Check declared allowlist.
	_, ok := cs.allowedHosts[host]
	return ok
}
