package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGroupSpecJSONRoundTrip(t *testing.T) {
	gs := GroupSpec{
		Group:    "239.1.1.1",
		Port:     5000,
		Iface:    "eth0",
		TTL:      1,
		Symbol:   "AAPL",
		SourceIP: "10.0.0.1",
	}

	data, err := json.Marshal(gs)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded GroupSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded != gs {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", decoded, gs)
	}
}

func TestGroupSpecJSONFieldNames(t *testing.T) {
	gs := GroupSpec{
		Group:    "239.2.2.2",
		SourceIP: "10.0.0.5",
	}

	data, err := json.Marshal(gs)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	raw := string(data)

	// Verify snake_case field names appear in JSON output
	for _, want := range []string{`"group"`, `"port"`, `"iface"`, `"ttl"`, `"symbol"`, `"source_ip"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("JSON output missing field %s; got: %s", want, raw)
		}
	}
}
