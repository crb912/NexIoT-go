# Go Performance Reference

Extended reference for profiling, benchmarking, and low-level optimisation.
Linked from SKILL.md — read that first.

---

## Profiling workflow

### 1. CPU profile

```go
import "runtime/pprof"

f, _ := os.Create("cpu.prof")
pprof.StartCPUProfile(f)
defer pprof.StopCPUProfile()
// ... run workload ...
```

Or in tests (no code change needed):

```bash
go test -bench=BenchmarkFoo -cpuprofile=cpu.prof
go tool pprof -http=:6060 cpu.prof   # open browser UI
```

Key views: `top` (hottest functions), `list FuncName` (annotated source), `web` (call graph).

### 2. Memory / allocation profile

```bash
go test -bench=BenchmarkFoo -memprofile=mem.prof -benchmem
go tool pprof -alloc_objects mem.prof   # count of allocations
go tool pprof -alloc_space   mem.prof   # bytes allocated
```

Look for unexpected allocations in hot paths — each heap allocation costs ~25 ns + GC pressure.

### 3. Execution trace (goroutine scheduling)

```go
import "runtime/trace"

f, _ := os.Create("trace.out")
trace.Start(f)
defer trace.Stop()
```

```bash
go tool trace trace.out
```

Use to diagnose goroutine starvation, excessive GC stop-the-world pauses, or scheduler latency.

### 4. HTTP live profiling (production-safe)

```go
import _ "net/http/pprof"
// Registers handlers on default ServeMux.
go http.ListenAndServe(":6060", nil)
```

```bash
go tool pprof http://localhost:6060/debug/pprof/heap
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

Restrict this endpoint to internal networks or behind auth in production.

---

## Escape analysis

```bash
go build -gcflags='-m' ./...      # single-level: shows what escapes
go build -gcflags='-m=2' ./...    # verbose: shows why
```

Read the output:
- `does not escape` — stack-allocated, no GC pressure. Good.
- `escapes to heap` — heap-allocated. Investigate if in a hot path.
- `inlining call to Foo` — function inlined. Usually good for small helpers.

Common escape causes:
- Returning a pointer to a local variable
- Storing into an interface (`any`)
- Closures capturing a variable by address
- Appending to a slice whose backing array may need to grow

---

## Benchmarking template

```go
package mypkg_test

import (
    "testing"
)

// BenchmarkProcess measures the hot path under realistic input.
func BenchmarkProcess(b *testing.B) {
    input := makeRealisticInput()   // set up outside the loop

    b.ResetTimer()                  // exclude setup from measurement
    b.ReportAllocs()                // print allocs/op in output

    for i := 0; i < b.N; i++ {
        // Reset any reused buffers here, not inside the measured call.
        _ = Process(input)
    }
}

// BenchmarkProcessParallel measures throughput under concurrency.
func BenchmarkProcessParallel(b *testing.B) {
    input := makeRealisticInput()
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _ = Process(input)
        }
    })
}
```

Run and compare:
```bash
# Baseline
go test -bench=BenchmarkProcess -benchmem -count=5 | tee old.txt

# After change
go test -bench=BenchmarkProcess -benchmem -count=5 | tee new.txt

# Statistical comparison (install once)
go install golang.org/x/perf/cmd/benchstat@latest
benchstat old.txt new.txt
```

Interpret output:
- `ns/op` — latency per call
- `B/op` — bytes allocated per call (lower is better)
- `allocs/op` — heap allocations per call (0 is the goal for hot paths)

---

## sync.Pool patterns and pitfalls

### Correct get/put cycle

```go
var pool = sync.Pool{New: func() any { return &MyStruct{} }}

func handle() {
    obj := pool.Get().(*MyStruct)
    obj.reset()             // MUST reset before use — pool objects carry stale state
    defer pool.Put(obj)     // return even on error paths

    obj.doWork()
}
```

### Pooling byte slices (pointer-to-slice idiom)

Pool a `*[]byte`, not `[]byte`. Pooling a slice value loses the backing array on Put.

```go
var slicePool = sync.Pool{
    New: func() any {
        b := make([]byte, 0, 8192)
        return &b
    },
}

func compress(src []byte) []byte {
    bp := slicePool.Get().(*[]byte)
    buf := (*bp)[:0]

    buf = doCompress(buf, src)
    result := append([]byte(nil), buf...) // copy out

    *bp = buf
    slicePool.Put(bp)
    return result
}
```

### When NOT to use sync.Pool

- Object is small (< 256 B) and allocation rate is low — pooling adds lock overhead
- Object lifecycle is bounded to a single goroutine — use a local variable instead
- Object contains `sync.Mutex` or channels — safe, but think carefully about reset
- You need deterministic memory usage — Pool's GC behaviour is not guaranteed

---

## strings.Builder and bytes.Buffer

Prefer `strings.Builder` for string output (zero-copy `String()` method):

```go
var sb strings.Builder
sb.Grow(estimatedLen)   // pre-allocate when length is roughly known
for _, s := range parts {
    sb.WriteString(s)
    sb.WriteByte(',')
}
result := sb.String()   // no copy; shares backing array until sb is modified again
```

Pool a `*strings.Builder` for concurrent use:

```go
var sbPool = sync.Pool{New: func() any { return &strings.Builder{} }}

func build(parts []string) string {
    sb := sbPool.Get().(*strings.Builder)
    sb.Reset()
    defer sbPool.Put(sb)
    for _, p := range parts {
        sb.WriteString(p)
    }
    return sb.String()
}
```

---

## io.Reader / io.Writer chain optimisation

- Wrap any `io.Reader` in `bufio.NewReader` before repeated small `Read` calls
- Wrap any `io.Writer` in `bufio.NewWriter` before repeated small `Write` calls; flush explicitly
- Use `io.Copy` — it uses an internal 32 KB buffer and avoids intermediate allocations
- Implement `io.WriterTo` / `io.ReaderFrom` on your types to let `io.Copy` short-circuit the buffer

```go
// Efficient pipeline: src → gzip → sha256 → dst
r := bufio.NewReaderSize(src, 64*1024)
gz, _ := gzip.NewReader(r)
h := sha256.New()
w := bufio.NewWriterSize(io.MultiWriter(dst, h), 64*1024)
io.Copy(w, gz)
w.Flush()
checksum := h.Sum(nil)
```

---

## unsafe patterns (use sparingly)

Only cross this boundary when profiling confirms the allocation/copy cost is unacceptable.

```go
import "unsafe"

// Convert []byte → string without copy (read-only use only).
func bytesToString(b []byte) string {
    return unsafe.String(unsafe.SliceData(b), len(b))
}

// Convert string → []byte without copy (must never write to result).
func stringToBytes(s string) []byte {
    return unsafe.Slice(unsafe.StringData(s), len(s))
}
```

These are safe in Go 1.20+ as long as the underlying memory is not mutated through the alias.
Never store the result past the lifetime of the original value.
