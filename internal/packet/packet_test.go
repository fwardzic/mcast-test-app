package packet

import (
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	original := &Packet{
		Sequence:  42,
		Group:     "239.1.1.1",
		Source:    "10.0.0.1",
		Symbol:    "AAPL",
		Timestamp: Now(),
		Payload:   "AAPL 182.35 +0.12",
	}

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Sequence != original.Sequence {
		t.Errorf("Sequence: got %d, want %d", decoded.Sequence, original.Sequence)
	}
	if decoded.Group != original.Group {
		t.Errorf("Group: got %q, want %q", decoded.Group, original.Group)
	}
	if decoded.Source != original.Source {
		t.Errorf("Source: got %q, want %q", decoded.Source, original.Source)
	}
	if decoded.Symbol != original.Symbol {
		t.Errorf("Symbol: got %q, want %q", decoded.Symbol, original.Symbol)
	}
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp: got %q, want %q", decoded.Timestamp, original.Timestamp)
	}
	if decoded.Payload != original.Payload {
		t.Errorf("Payload: got %q, want %q", decoded.Payload, original.Payload)
	}
}

func TestEncodeSnakeCaseFields(t *testing.T) {
	p := &Packet{
		Sequence:  1,
		Group:     "239.1.1.1",
		Source:    "10.0.0.1",
		Symbol:    "GOOG",
		Timestamp: "2026-04-20T12:34:56.789Z",
		Payload:   "GOOG 2847.12 -3.45",
	}

	data, err := Encode(p)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	raw := string(data)
	for _, want := range []string{`"sequence"`, `"group"`, `"source"`, `"symbol"`, `"timestamp"`, `"payload"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("JSON missing snake_case field %s; got: %s", want, raw)
		}
	}
}

func TestDecodeInvalidJSON(t *testing.T) {
	_, err := Decode([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestNowFormat(t *testing.T) {
	ts := Now()
	// RFC 3339 with millis: "2026-04-20T12:34:56.789Z"
	if len(ts) != 24 {
		t.Errorf("Now() length: got %d, want 24; value: %q", len(ts), ts)
	}
	if !strings.HasSuffix(ts, "Z") {
		t.Errorf("Now() should end with Z; got: %q", ts)
	}
	if ts[10] != 'T' {
		t.Errorf("Now() should have T at position 10; got: %q", ts)
	}
}
