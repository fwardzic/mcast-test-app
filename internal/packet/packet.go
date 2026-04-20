// Package packet defines the wire format for multicast test messages.
//
// We use JSON encoding (not protobuf or a binary format) deliberately:
// a learner can read packets in tcpdump -A output without any decoder tool.
// The trade-off is larger packets and slower marshalling, which is fine for
// a test tool sending a few hundred packets per second.
package packet

import (
	"encoding/json"
	"time"
)

// Packet is the payload sent in every multicast UDP datagram.
type Packet struct {
	// Sequence is a monotonically increasing counter per sender goroutine.
	// The receiver uses it to detect gaps (lost packets).
	Sequence uint64 `json:"sequence"`

	// Group is the multicast destination address this packet was sent to.
	Group string `json:"group"`

	// Source is the unicast IP of the sender (useful for SSM identification).
	Source string `json:"source"`

	// Symbol is a short ticker name (e.g., "AAPL") identifying the data stream.
	Symbol string `json:"symbol"`

	// Timestamp is when the sender created the packet (RFC 3339, millisecond precision).
	// The receiver can subtract this from its wall clock for a rough one-way delay estimate.
	Timestamp string `json:"timestamp"`

	// Payload is a human-readable string simulating market data, e.g. "AAPL 182.35 +0.12".
	Payload string `json:"payload"`
}

// Now returns the current time formatted as RFC 3339 with millisecond precision.
// Used by the sender to stamp each packet.
func Now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// Encode serialises a Packet to JSON bytes for transmission.
func Encode(p *Packet) ([]byte, error) {
	return json.Marshal(p)
}

// Decode deserialises JSON bytes back into a Packet.
func Decode(data []byte) (*Packet, error) {
	var p Packet
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
