package display

import (
	"bytes"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	var buf bytes.Buffer
	if err := Init(&buf, 5); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	got := buf.String()

	// Must contain hide-cursor and scroll-region escape sequences.
	if !strings.Contains(got, "\033[?25l") {
		t.Error("Init output missing hideCursor escape")
	}
	if !strings.Contains(got, "\033[1;5r") {
		t.Error("Init output missing scroll region escape")
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name         string
		lines        []string
		status       string
		scrollRows   int
		wantContains []string
	}{
		{
			name:         "single line",
			lines:        []string{"hello"},
			status:       "0 pkts",
			scrollRows:   5,
			wantContains: []string{"hello", "0 pkts"},
		},
		{
			name:         "multiple lines",
			lines:        []string{"line1", "line2"},
			status:       "stats",
			scrollRows:   5,
			wantContains: []string{"line1", "line2", "stats"},
		},
		{
			name:         "empty lines",
			lines:        []string{},
			status:       "idle",
			scrollRows:   5,
			wantContains: []string{"idle"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Render(&buf, tt.lines, tt.status, tt.scrollRows)
			got := buf.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q", want)
				}
			}
		})
	}
}

func TestTeardown(t *testing.T) {
	var buf bytes.Buffer
	Teardown(&buf, 24)
	got := buf.String()

	if !strings.Contains(got, "\033[r") {
		t.Error("Teardown output missing resetScroll escape")
	}
	if !strings.Contains(got, "\033[?25h") {
		t.Error("Teardown output missing showCursor escape")
	}
}

func TestRingBuf(t *testing.T) {
	tests := []struct {
		name     string
		cap      int
		pushVals []string
		want     []string
	}{
		{
			name:     "under capacity",
			cap:      5,
			pushVals: []string{"a", "b"},
			want:     []string{"", "", "", "a", "b"},
		},
		{
			name:     "at capacity",
			cap:      5,
			pushVals: []string{"a", "b", "c", "d", "e"},
			want:     []string{"a", "b", "c", "d", "e"},
		},
		{
			name:     "overflow wraps",
			cap:      5,
			pushVals: []string{"a", "b", "c", "d", "e", "f", "g"},
			want:     []string{"c", "d", "e", "f", "g"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRingBuf(tt.cap)
			for _, v := range tt.pushVals {
				r.Push(v)
			}
			got := r.Ordered()
			if len(got) != len(tt.want) {
				t.Fatalf("Ordered() len = %d, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("Ordered()[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

func TestColorize(t *testing.T) {
	got := Colorize("x", 33)
	want := "\033[33mx\033[0m"
	if got != want {
		t.Errorf("Colorize(\"x\", 33) = %q, want %q", got, want)
	}
}

func TestGroupColor(t *testing.T) {
	if got := GroupColor(0); got != 33 {
		t.Errorf("GroupColor(0) = %d, want 33", got)
	}
	// Index 5 should wrap back to palette[0] = 33.
	if got := GroupColor(5); got != 33 {
		t.Errorf("GroupColor(5) = %d, want 33", got)
	}
}
