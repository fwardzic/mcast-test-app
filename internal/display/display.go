// Package display renders a fixed-height ANSI scrolling terminal UI for the
// multicast receiver. It owns all escape sequences; callers never write raw
// ANSI codes.
//
// TTY detection and terminal size queries are the caller's responsibility
// (see cmd/receiver/main.go). display.Init must be called before Render,
// and display.Teardown must be deferred in main() to restore the terminal.
package display

import (
	"bytes"
	"fmt"
	"io"
)

// ANSI escape sequences used to control terminal output.
// These are unexported because callers should use Init/Render/Teardown instead.
const (
	hideCursor  = "\033[?25l"
	showCursor  = "\033[?25h"
	eraseEOL    = "\033[2K"
	resetScroll = "\033[r"
)

// moveTo returns an ANSI escape that moves the cursor to the given row and column.
func moveTo(row, col int) string {
	return fmt.Sprintf("\033[%d;%dH", row, col)
}

// setScrollRegion returns an ANSI escape that restricts scrolling to rows top..bot.
func setScrollRegion(top, bot int) string {
	return fmt.Sprintf("\033[%d;%dr", top, bot)
}

// Colorize wraps s with the given ANSI color code and a reset suffix.
// colorCode should be a full SGR parameter (e.g. 33 for yellow foreground).
func Colorize(s string, colorCode int) string {
	return fmt.Sprintf("\033[%dm%s\033[0m", colorCode, s)
}

// colorPalette cycles through 5 ANSI foreground colors that are visible on
// both dark and light terminal backgrounds.
var colorPalette = []int{33, 36, 32, 35, 34} // yellow, cyan, green, magenta, blue

// GroupColor returns the ANSI color code for the given group index.
// It wraps around the palette so any number of groups can be colored.
func GroupColor(groupIndex int) int {
	return colorPalette[groupIndex%len(colorPalette)]
}

// RingBuf is a fixed-capacity circular buffer of strings.
// Used to hold the most recent N display lines for the scroll region.
type RingBuf struct {
	data []string
	head int // next write position (monotonically increasing)
	cap  int
}

// NewRingBuf creates a ring buffer that holds at most cap strings.
func NewRingBuf(cap int) *RingBuf {
	return &RingBuf{data: make([]string, cap), cap: cap}
}

// Push adds a string to the ring buffer, overwriting the oldest entry if full.
func (r *RingBuf) Push(s string) {
	r.data[r.head%r.cap] = s
	r.head++
}

// Ordered returns all lines in oldest-to-newest order.
// Empty slots (not yet written) appear as empty strings.
func (r *RingBuf) Ordered() []string {
	out := make([]string, r.cap)
	for i := 0; i < r.cap; i++ {
		out[i] = r.data[(r.head+i)%r.cap]
	}
	return out
}

// Init hides the cursor and sets the scroll region to rows 1..scrollRows.
// Writes are buffered and flushed atomically to minimise flicker.
func Init(out io.Writer, scrollRows int) error {
	var buf bytes.Buffer
	buf.WriteString(hideCursor)
	buf.WriteString(setScrollRegion(1, scrollRows))
	_, err := out.Write(buf.Bytes())
	return err
}

// Render writes all ring-buffer lines into the scroll region and the status
// string on the line below. Buffers all output for one atomic write.
func Render(out io.Writer, lines []string, status string, scrollRows int) {
	var buf bytes.Buffer
	for i, line := range lines {
		buf.WriteString(moveTo(i+1, 1))
		buf.WriteString(eraseEOL)
		buf.WriteString(line)
	}
	// Status line sits just below the scroll region.
	buf.WriteString(moveTo(scrollRows+1, 1))
	buf.WriteString(eraseEOL)
	buf.WriteString(status)
	// Leave cursor at bottom of scroll region so new terminal output appends.
	buf.WriteString(moveTo(scrollRows, 1))
	out.Write(buf.Bytes()) //nolint:errcheck -- best-effort terminal write
}

// Teardown resets the scroll region and shows the cursor. Must be deferred
// from main() after a successful Init call.
func Teardown(out io.Writer, totalRows int) {
	var buf bytes.Buffer
	buf.WriteString(resetScroll)
	buf.WriteString(moveTo(totalRows+1, 1))
	buf.WriteString(showCursor)
	out.Write(buf.Bytes()) //nolint:errcheck -- best-effort terminal write
}
