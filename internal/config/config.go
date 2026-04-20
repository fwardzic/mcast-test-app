// Package config defines shared configuration types for the multicast test application.
// These types are used by both the sender and receiver to describe multicast group parameters.
package config

// GroupSpec describes a single multicast group that the sender or receiver will operate on.
// Each field maps to a CLI flag or config value; see cmd/sender and cmd/receiver for usage.
type GroupSpec struct {
	// Group is the multicast destination address (e.g., "239.1.1.1").
	Group string `json:"group"`

	// Port is the UDP destination port.
	Port int `json:"port"`

	// Iface is the network interface name to bind to (e.g., "eth0").
	// An empty string means the OS picks the default multicast interface.
	Iface string `json:"iface"`

	// TTL is the IP multicast TTL (time-to-live / hop limit). Typical LAN value: 1.
	TTL int `json:"ttl"`

	// Symbol is the ticker symbol this group carries (e.g., "AAPL").
	// Used by the sender to stamp each packet with a human-readable identifier.
	Symbol string `json:"symbol"`

	// SourceIP is the source address for SSM (Source-Specific Multicast) joins.
	// Leave empty for ASM (Any-Source Multicast). When set, the receiver uses
	// IGMPv3 source filtering to accept packets only from this source.
	SourceIP string `json:"source_ip"`
}

// Defaults for CLI flags. Centralised here so sender and receiver share the same values.
const (
	DefaultGroup = "239.1.1.1"
	DefaultPort  = 5000
	DefaultTTL   = 1
	DefaultRate  = 1 // packets per second
)
