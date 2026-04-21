# Learning Go Through This Codebase

A guide for Python developers who want to understand how this multicast testing app works — and learn Go concepts along the way.

---

## Table of Contents

1. [The Big Picture](#the-big-picture)
2. [Entry Points: main() is Your if __name__ == "__main__"](#entry-points)
3. [Packages: Go's Version of Python Modules](#packages)
4. [Structs: Go's Version of Classes (Sort Of)](#structs)
5. [Methods: Functions Attached to Structs](#methods)
6. [Interfaces: The Duck Typing You Already Know](#interfaces)
7. [Goroutines: Lightweight Threads That Just Work](#goroutines)
8. [Channels: How Goroutines Talk to Each Other](#channels)
9. [The select Statement: Multiplexing Channels](#the-select-statement)
10. [Context: The Cancellation Signal](#context)
11. [sync.WaitGroup: Waiting for Goroutines to Finish](#syncwaitgroup)
12. [sync.Once: Do Something Exactly Once](#synconce)
13. [Error Handling: No Exceptions Here](#error-handling)
14. [Putting It All Together: Data Flow](#putting-it-all-together)

---

## The Big Picture

This app has two programs:

- **Sender** (`cmd/sender/main.go`) — sends fake market data packets to multicast groups
- **Receiver** (`cmd/receiver/main.go`) — listens for those packets, tracks gaps, shows live stats

Think of it like a radio station (sender) broadcasting on frequencies (multicast groups) and a radio (receiver) tuning in.

```
┌──────────┐    UDP multicast    ┌───────────┐
│  Sender  │ ──────────────────> │  Receiver │
│          │   239.1.1.1:5000    │           │
│ sends    │                     │ receives  │
│ packets  │                     │ tracks    │
│ at N/sec │                     │ gaps      │
└──────────┘                     └───────────┘
```

The supporting code lives in `internal/`:
- `packet/` — defines what a packet looks like and how to serialize it
- `config/` — shared configuration types
- `multicast/` — socket wrappers for sending/receiving
- `display/` — terminal UI for the receiver

---

## Entry Points

In Python you write:
```python
if __name__ == "__main__":
    main()
```

In Go, the entry point is always `func main()` in `package main`. This project has two:

```
cmd/sender/main.go    → builds into the "sender" binary
cmd/receiver/main.go  → builds into the "receiver" binary
```

Here's the sender's `main()` stripped to its skeleton:

```go
func main() {
    // 1. Parse CLI flags (like argparse)
    groups := pflag.StringArrayP("group", "g", []string{config.DefaultGroup}, "...")
    pflag.Parse()

    // 2. Validate inputs — exit early if bad
    if err := validateFlags(*groups, *iface); err != nil {
        slog.Error("bad flags", "error", err)
        os.Exit(1)
    }

    // 3. Set up signal handling (Ctrl-C)
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // 4. Create the network connection
    sc, err := multicast.NewSenderConn(*iface, *ttl, *loopback)
    if err != nil {
        slog.Error("socket failed", "error", err)
        os.Exit(1)
    }
    defer sc.Close()

    // 5. Launch one goroutine per group
    var wg sync.WaitGroup
    for i, grp := range *groups {
        wg.Add(1)
        go sendLoop(ctx, sc, dst, spec, *rate)
    }

    // 6. Wait for all goroutines to finish
    wg.Wait()
}
```

**Python equivalent in spirit:**
```python
def main():
    args = parse_args()
    validate(args)
    sock = create_socket(args)
    threads = [Thread(target=send_loop, args=(...)) for g in args.groups]
    for t in threads: t.start()
    for t in threads: t.join()
```

The structure is the same. The difference is in how Go handles the details.

---

## Packages

In Python, a folder with `__init__.py` is a package. In Go, a folder is a package, and every `.go` file in that folder starts with `package <name>`.

```
internal/
  packet/
    packet.go      → package packet
    packet_test.go  → package packet
  config/
    config.go      → package config
  multicast/
    sender.go      → package multicast
    receiver.go    → package multicast   ← same package, different files
    receiver_conn.go → package multicast
```

**Key differences from Python:**

| Python | Go |
|--------|-----|
| `from packet import Packet` | `import "github.com/fwardzic/mcast-test-app/internal/packet"` then use `packet.Packet` |
| Lowercase = private by convention | Lowercase = private **enforced by the compiler** |
| Uppercase = public by convention | Uppercase = public **enforced by the compiler** |

In this codebase, `packet.Encode()` is public (starts with uppercase E), but if there were a `helper()` function starting with lowercase, it would be invisible outside the package.

The `internal/` directory is special in Go: code under `internal/` can only be imported by code in the parent tree. It's Go's way of saying "these are private implementation details of this module."

---

## Structs

Python has classes. Go has structs. They hold data but don't have inheritance.

### Simple data container: Packet

```go
// internal/packet/packet.go
type Packet struct {
    Sequence  uint64 `json:"sequence"`
    Group     string `json:"group"`
    Source    string `json:"source"`
    Symbol    string `json:"symbol"`
    Timestamp string `json:"timestamp"`
    Payload   string `json:"payload"`
}
```

**Python equivalent:**
```python
@dataclass
class Packet:
    sequence: int
    group: str
    source: str
    symbol: str
    timestamp: str
    payload: str
```

The backtick tags (`json:"sequence"`) tell Go's JSON encoder what field names to use. Without them, it would use "Sequence" (uppercase) in JSON.

### Struct with behavior: ReceiverConn

```go
// internal/multicast/receiver_conn.go
type ReceiverConn struct {
    pc        *ipv4.PacketConn
    conn      net.PacketConn
    closeOnce sync.Once
    closeErr  error
}
```

Notice the lowercase field names — they're private. Nothing outside the `multicast` package can access `rc.pc` directly. This is like Python's `self._pc` convention, except Go actually enforces it.

### Struct with state tracking: GroupStats

```go
// cmd/receiver/main.go
type GroupStats struct {
    PktCount uint64
    GapCount uint64
    LastSeq  uint64
    FirstPkt bool
}
```

This tracks per-group statistics in the receiver. In Python you might use a dict or a class — in Go, you define a struct. The receiver creates a map of these:

```go
stats := make(map[string]*GroupStats)  // Python: stats: dict[str, GroupStats] = {}
```

### Circular buffer: RingBuf

```go
// internal/display/display.go
type RingBuf struct {
    data []string
    head int
    cap  int
}
```

A fixed-size circular buffer for the scrolling terminal display. When it's full, new entries overwrite the oldest. The `head` index keeps incrementing and wraps via modulo:

```go
func (r *RingBuf) Push(s string) {
    r.data[r.head%r.cap] = s  // Wrap around
    r.head++
}
```

---

## Methods

In Python, methods live inside the class. In Go, methods are functions with a **receiver** — the struct they belong to.

```go
// Python way:
// class SenderConn:
//     def write_to(self, b: bytes, dst) -> int:
//         return self.pc.write_to(b, dst)

// Go way:
func (s *SenderConn) WriteTo(b []byte, dst net.Addr) (int, error) {
    return s.pc.WriteTo(b, 0, nil, dst)
}
```

The `(s *SenderConn)` before the function name is the receiver. It's like `self` in Python, but:
- You choose the name (convention: first letter of the type, so `s` for SenderConn)
- The `*` means it's a pointer (you're modifying the original, not a copy)

**Constructor pattern** — Go doesn't have `__init__`. By convention, you write a `New___()` function:

```go
func NewSenderConn(ifaceName string, ttl int, loopback bool) (*SenderConn, error) {
    // ... set up socket ...
    return &SenderConn{pc: pc, conn: conn}, nil
}
```

The `&` creates a pointer to the struct. This is like returning `self` from `__init__`, except Go makes the allocation explicit.

---

## Interfaces

This is where Go gets interesting, especially for Python developers.

In Python, duck typing means "if it quacks like a duck, it's a duck." Go has the same philosophy, but **checked at compile time**.

### PacketWriter — abstracting "something that can send bytes"

```go
// internal/multicast/sender.go
type PacketWriter interface {
    WriteTo(b []byte, dst net.Addr) (int, error)
    Close() error
}
```

This says: "anything that has `WriteTo` and `Close` methods with these exact signatures is a `PacketWriter`." You never write `implements PacketWriter` anywhere. If your struct has these methods, it automatically satisfies the interface.

`SenderConn` satisfies `PacketWriter` because it has both methods:

```go
func (s *SenderConn) WriteTo(b []byte, dst net.Addr) (int, error) { ... }
func (s *SenderConn) Close() error { ... }
```

**Why bother?** Look at `sendLoop`:

```go
func sendLoop(ctx context.Context, w PacketWriter, dst net.Addr, spec config.GroupSpec, pps int) {
    // w could be a real SenderConn or a fake for testing
}
```

In tests, they pass a mock:

```go
type mockWriter struct {
    packets [][]byte
    mu      sync.Mutex
}

func (m *mockWriter) WriteTo(b []byte, _ net.Addr) (int, error) {
    m.mu.Lock()
    m.packets = append(m.packets, append([]byte(nil), b...))
    m.mu.Unlock()
    return len(b), nil
}

func (m *mockWriter) Close() error { return nil }
```

This mock also satisfies `PacketWriter` — no inheritance, no registration, just matching methods.

**Python equivalent:**
```python
# Python uses Protocol (or ABC) for the same thing
class PacketWriter(Protocol):
    def write_to(self, b: bytes, dst) -> tuple[int, Exception | None]: ...
    def close(self) -> Exception | None: ...
```

### IGMPConn — abstracting multicast group operations

```go
type IGMPConn interface {
    JoinGroup(ifi *net.Interface, group net.Addr) error
    LeaveGroup(ifi *net.Interface, group net.Addr) error
    JoinSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error
    LeaveSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error
}
```

The real implementation is `*ipv4.PacketConn` from the `golang.org/x/net` library. The tests use a mock. Same pattern.

---

## Goroutines

A goroutine is a lightweight thread. In Python, you'd use `threading.Thread` or `asyncio`. In Go, you just put `go` in front of a function call.

### Sender: one goroutine per group

```go
// cmd/sender/main.go
for i, grp := range *groups {
    wg.Add(1)
    go sendLoop(ctx, sc, dst, spec, *rate)
}
```

Each group gets its own goroutine running `sendLoop`. If you have 3 groups, you get 3 goroutines all sending packets concurrently.

**Python equivalent:**
```python
for grp in groups:
    t = Thread(target=send_loop, args=(ctx, sc, dst, spec, rate))
    t.start()
```

But goroutines are much cheaper than threads — you can run thousands of them. The Go runtime multiplexes them onto OS threads for you.

### sendLoop: the goroutine's work

```go
func sendLoop(ctx context.Context, w PacketWriter, dst net.Addr, spec config.GroupSpec, pps int) {
    ticker := time.NewTicker(time.Second / time.Duration(pps))
    defer ticker.Stop()
    var seq uint64

    for {
        select {
        case <-ctx.Done():
            return  // Signal received, exit cleanly
        case <-ticker.C:
            seq++
            pkt := &packet.Packet{
                Sequence: seq,
                Group:    spec.Group,
                // ...
            }
            data, err := packet.Encode(pkt)
            if err != nil {
                slog.Error("encode failed", "error", err)
                continue
            }
            if _, err := w.WriteTo(data, dst); err != nil {
                slog.Error("send failed", "error", err)
            }
        }
    }
}
```

This loop runs forever until `ctx.Done()` fires (from Ctrl-C). The `ticker` fires every `1/pps` seconds, triggering a packet send.

### Receiver: a pipeline of 4 goroutines

The receiver is more complex. Here's the goroutine architecture:

```
Signal (Ctrl-C)
    │
    ▼
┌─────────────────┐
│ Watcher goroutine│──── closes socket to unblock ReadFrom
└─────────────────┘
    │
    ▼
┌─────────────────┐         ┌──────────────────┐         ┌──────────────────┐
│  receiveLoop    │─────────│  groupManager    │─────────│  displayLoop     │
│                 │ packetCh│                  │ linesCh │                  │
│ reads UDP       │────────>│ tracks gaps      │────────>│ renders terminal │
│ decodes JSON    │         │ calculates rates │ statsCh │ at 10 Hz         │
│                 │         │                  │────────>│                  │
└─────────────────┘         └──────────────────┘         └──────────────────┘
```

Each box is a goroutine. The arrows are channels. This is Go's signature pattern: **concurrent pipeline**.

---

## Channels

Channels are typed pipes for communication between goroutines. Python has `queue.Queue` — channels are similar but built into the language.

### Creating channels

```go
packetCh := make(chan ReceivedPacket, 64)   // buffered: holds up to 64 items
linesCh  := make(chan string, 64)           // buffered: holds up to 64 items
statsCh  := make(chan string, 4)            // buffered: holds up to 4 items
```

**Python equivalent:**
```python
packet_queue = queue.Queue(maxsize=64)
lines_queue = queue.Queue(maxsize=64)
```

The buffer size matters:
- **Unbuffered** (`make(chan T)`) — sender blocks until receiver is ready (like a handshake)
- **Buffered** (`make(chan T, 64)`) — sender can put up to 64 items without blocking

### Sending and receiving

```go
// Sending (in receiveLoop):
packetCh <- rp           // Put rp into the channel

// Receiving (in groupManager):
for rp := range packetCh {   // Read until channel is closed
    // process rp
}
```

The `range` over a channel is like iterating a queue — it blocks waiting for the next item, and exits when the channel is closed.

### Closing channels

```go
// In receiveLoop:
defer close(packetCh)  // Tells groupManager "no more packets coming"
```

Closing a channel signals all readers that no more data will arrive. The `range` loop exits. This is how the pipeline shuts down cleanly:

1. Signal arrives → context cancelled
2. Watcher closes socket → `ReadFrom` returns error
3. `receiveLoop` returns → `close(packetCh)`
4. `groupManager` sees closed channel → leaves groups → `close(linesCh)` and `close(statsCh)`
5. `displayLoop` sees closed channels → returns

### Non-blocking sends

Sometimes you don't want to block if the receiver is slow:

```go
// In groupManager:
select {
case linesCh <- formattedLine:
    // sent successfully
default:
    // channel full, drop the line — display will catch up
}
```

This is like `queue.put_nowait()` in Python. It prevents a slow terminal display from blocking packet processing.

### Channel direction in function signatures

Go lets you restrict what a function can do with a channel:

```go
func receiveLoop(ctx context.Context, pc *ipv4.PacketConn, packetCh chan<- ReceivedPacket, ...) {
    // chan<- means "send only" — this function can put items in but can't read from packetCh
}

func displayLoop(ctx context.Context, linesCh <-chan string, statsCh <-chan string, ...) {
    // <-chan means "receive only" — this function can read but can't send or close
}
```

This is compile-time documentation of intent. The compiler enforces it.

---

## The select Statement

`select` is Go's way of waiting on multiple channels at once. It's like `asyncio.wait` but for channels.

### In sendLoop — wait for either a tick or cancellation:

```go
for {
    select {
    case <-ctx.Done():
        return  // Stop sending
    case <-ticker.C:
        // Send a packet
    }
}
```

Whichever channel has data first wins. If both are ready, Go picks one randomly.

### In displayLoop — wait for lines, stats, timer, resize, or cancellation:

```go
for {
    select {
    case line, ok := <-linesCh:
        if !ok { return }     // Channel closed
        ring.Push(line)
    case s, ok := <-statsCh:
        if ok { status = s }
    case <-ticker.C:
        display.Render(os.Stdout, ring.Ordered(), status, scrollRows)
    case <-sigCh:
        // Terminal resized — recalculate layout
    case <-ctx.Done():
        return
    }
}
```

The `ok` in `line, ok := <-linesCh` tells you if the channel is still open. `ok == false` means closed.

**Python equivalent (conceptually):**
```python
# There's no clean Python equivalent. The closest is:
done, pending = await asyncio.wait(
    [lines_task, stats_task, tick_task, signal_task],
    return_when=FIRST_COMPLETED
)
```

---

## Context

`context.Context` is Go's way of propagating cancellation signals through a call tree. It solves the problem of "how do I tell 10 goroutines to stop when the user presses Ctrl-C?"

```go
// Create a context that cancels on SIGINT or SIGTERM:
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()  // Clean up signal registration

// Pass ctx to every goroutine:
go sendLoop(ctx, sc, dst, spec, rate)
```

Inside `sendLoop`:
```go
case <-ctx.Done():
    return  // ctx.Done() is a channel that closes when cancelled
```

**Python equivalent:**
```python
# Python uses threading.Event or asyncio.Event
shutdown = threading.Event()
signal.signal(signal.SIGINT, lambda *_: shutdown.set())

# In the loop:
if shutdown.is_set():
    return
```

Context is more powerful because it chains — you can create child contexts with timeouts, and cancelling a parent cancels all children.

### The watcher goroutine trick

The receiver has a subtle problem: `pc.ReadFrom()` blocks forever waiting for network data. Context cancellation won't interrupt it. Solution:

```go
go func() {
    <-ctx.Done()    // Wait for Ctrl-C
    rc.Close()      // Close the socket — this makes ReadFrom() return an error
}()
```

Then in `receiveLoop`:
```go
n, cm, _, err := pc.ReadFrom(buf)
if err != nil {
    if ctx.Err() != nil {
        return  // Error was caused by our intentional close — clean shutdown
    }
    return  // Unexpected error
}
```

This is a common Go pattern for unblocking I/O operations during shutdown.

---

## sync.WaitGroup

A WaitGroup waits for a collection of goroutines to finish. It's a counter:

```go
var wg sync.WaitGroup

// Before launching each goroutine:
wg.Add(1)

// Inside each goroutine (first line):
defer wg.Done()  // Decrements the counter when goroutine exits

// In main, after launching all goroutines:
wg.Wait()  // Blocks until counter reaches 0
```

**Python equivalent:**
```python
threads = []
for g in groups:
    t = Thread(target=work)
    t.start()
    threads.append(t)
for t in threads:
    t.join()  # This is what wg.Wait() does
```

The `defer` keyword means "run this when the function returns." It's like a `finally` block but more concise. Multiple `defer` calls execute in LIFO (last-in, first-out) order.

---

## sync.Once

Sometimes you need to ensure something runs exactly once, even if called from multiple goroutines. `ReceiverConn.Close()` shows this:

```go
type ReceiverConn struct {
    closeOnce sync.Once
    closeErr  error
    // ...
}

func (r *ReceiverConn) Close() error {
    r.closeOnce.Do(func() {
        r.closeErr = r.conn.Close()
    })
    return r.closeErr
}
```

Without `sync.Once`, if the watcher goroutine and `receiveLoop` both call `Close()`, you'd close the socket twice (which could crash). `sync.Once` guarantees the inner function runs exactly once — all other callers just return the stored result.

**Python equivalent:**
```python
class ReceiverConn:
    def __init__(self):
        self._close_lock = threading.Lock()
        self._closed = False
        self._close_err = None

    def close(self):
        with self._close_lock:
            if not self._closed:
                self._close_err = self.conn.close()
                self._closed = True
        return self._close_err
```

Go's `sync.Once` packages this pattern into a single type.

---

## Error Handling

Go doesn't have exceptions. Functions return errors as values:

```go
sc, err := multicast.NewSenderConn(*iface, *ttl, *loopback)
if err != nil {
    slog.Error("socket failed", "error", err)
    os.Exit(1)
}
```

**Python equivalent:**
```python
try:
    sc = multicast.new_sender_conn(iface, ttl, loopback)
except Exception as e:
    logging.error(f"socket failed: {e}")
    sys.exit(1)
```

The `if err != nil` pattern is everywhere in Go. You'll see it after every function call that can fail. It's verbose compared to Python's try/except, but it makes error handling explicit — you can never forget to handle an error because the compiler forces you to use or explicitly ignore the return value.

This codebase uses three error strategies:

1. **Fatal** — bad flags, can't create socket → `os.Exit(1)`
2. **Skip** — can't decode a packet → `continue` (try the next one)
3. **Log and continue** — can't send a packet → log warning, keep going

---

## Putting It All Together

Here's how the entire receiver flows, annotated with the Go concepts used:

```go
func main() {
    // PACKAGES: pflag for CLI parsing
    groups := pflag.StringArrayP("group", "g", ...)
    pflag.Parse()

    // ERROR HANDLING: validate and exit early
    if err := validateFlags(...); err != nil {
        os.Exit(1)
    }

    // CONTEXT: cancellation signal for all goroutines
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // STRUCT + CONSTRUCTOR: create receiver connection
    rc, err := multicast.NewReceiverConn(*port)
    if err != nil { os.Exit(1) }
    defer rc.Close()  // DEFER: clean up on exit

    // INTERFACE: rc.PC() returns IGMPConn for group joins
    for _, spec := range specs {
        multicast.JoinASM(rc.PC(), ifi, groupIP)
    }

    // CHANNELS: typed pipes between goroutines
    packetCh := make(chan ReceivedPacket, 64)

    // GOROUTINE + SYNC.ONCE: watcher closes socket to unblock ReadFrom
    go func() {
        <-ctx.Done()
        rc.Close()  // sync.Once inside makes this safe
    }()

    // GOROUTINE + WAITGROUP: receive loop
    var wg sync.WaitGroup
    wg.Add(1)
    go receiveLoop(ctx, rc.PC(), packetCh, &wg)
    // Inside receiveLoop:
    //   - reads from network (blocking I/O)
    //   - STRUCT: creates ReceivedPacket
    //   - CHANNEL: sends on packetCh
    //   - closes packetCh on exit (signals downstream)

    // CHANNELS: display pipeline
    linesCh := make(chan string, 64)
    statsCh := make(chan string, 4)

    // GOROUTINE: display loop
    wg.Add(1)
    go displayLoop(ctx, linesCh, statsCh, scrollRows, &wg)
    // Inside displayLoop:
    //   - SELECT: waits on linesCh, statsCh, ticker, resize signal, ctx
    //   - STRUCT: uses RingBuf circular buffer

    // GOROUTINE: group manager (the brain)
    wg.Add(1)
    go groupManager(packetCh, linesCh, statsCh, specs, rc.PC(), ifi, &wg)
    // Inside groupManager:
    //   - CHANNEL: range over packetCh (blocks until data or close)
    //   - STRUCT: GroupStats for gap tracking
    //   - MAP: stats per group
    //   - SELECT: non-blocking sends to display channels
    //   - INTERFACE: calls LeaveASM/LeaveSSM on shutdown

    // WAITGROUP: wait for everything to finish
    wg.Wait()
}
```

### The shutdown sequence (what makes this non-trivial)

```
User presses Ctrl-C
    │
    ├─ context cancelled (ctx.Done() closes)
    │
    ├─ watcher goroutine: <-ctx.Done() fires → rc.Close()
    │                                              │
    │                                              ▼
    ├─ receiveLoop: ReadFrom() returns error
    │   checks ctx.Err() != nil → clean exit
    │   defer close(packetCh)
    │              │
    │              ▼
    ├─ groupManager: range packetCh exits (channel closed)
    │   leaves all multicast groups
    │   close(linesCh), close(statsCh)
    │              │
    │              ▼
    ├─ displayLoop: linesCh closed → returns
    │
    └─ wg.Wait() returns → main() exits
```

Every goroutine has a clear shutdown path. No goroutine leaks. This is the hardest part of concurrent programming, and this codebase handles it well.

---

## Quick Reference: Python to Go

| Python | Go | Where in this codebase |
|--------|-----|----------------------|
| `class Packet:` | `type Packet struct {}` | `internal/packet/packet.go` |
| `def __init__(self):` | `func NewXxx() *Xxx` | `multicast.NewSenderConn()` |
| `self.field` | `s.field` (receiver) | `func (s *SenderConn) WriteTo(...)` |
| `Protocol` / duck typing | `interface` | `PacketWriter`, `IGMPConn` |
| `threading.Thread(target=f).start()` | `go f()` | `go sendLoop(...)` |
| `queue.Queue()` | `make(chan T, size)` | `packetCh := make(chan ReceivedPacket, 64)` |
| `queue.put(x)` | `ch <- x` | `packetCh <- rp` |
| `queue.get()` | `x := <-ch` | `for rp := range packetCh` |
| `asyncio.wait(tasks)` | `select { case ... }` | `displayLoop`'s select block |
| `threading.Event` | `context.Context` | `ctx, stop := signal.NotifyContext(...)` |
| `thread.join()` | `wg.Wait()` | Throughout both `main()` functions |
| `try/except` | `if err != nil` | Every fallible function call |
| `finally:` | `defer` | `defer sc.Close()` |
| `import module` | `import "path/to/package"` | All files |
| private by `_` convention | private by lowercase | Struct fields like `pc`, `conn` |

---

## How Would You Write This From Scratch?

If you were building this yourself, here's the order of thinking:

1. **Define your data** — What does a packet look like? → `Packet` struct
2. **Define your operations** — What can you do? Send, receive, join groups → functions and methods
3. **Define your abstractions** — What might change? The network layer → `PacketWriter` interface
4. **Wire up concurrency** — What runs in parallel? → goroutines + channels
5. **Handle shutdown** — How does everything stop cleanly? → context + WaitGroup + channel closing
6. **Add the UI** — Terminal display on top → separate goroutine with its own render loop

The hardest part isn't any single concept — it's the shutdown choreography. Getting 4 goroutines to stop cleanly without deadlocks or panics is where the real engineering is.
