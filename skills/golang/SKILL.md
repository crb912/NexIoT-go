---
name: golang
description: >
  Use this skill when writing, reviewing, or refactoring Go (Golang) code.
  Triggers: go files, go.mod, goroutines, channels, interfaces, error handling,
  slice/map allocation, sync.Pool, hot-path optimization, or any task where
  the user mentions Go, Golang, gofmt, gopls, go build, go test, or go vet.
---

# Golang Skill

## Quick start

```go
// Canonical Go file layout
package main

import (
    "errors"
    "fmt"
)

// ErrNotFound is a sentinel — exported, declared at package level.
var ErrNotFound = errors.New("not found")

// User follows Go naming: no underscores, acronyms all-caps (UserID not UserId).
type User struct {
    ID   int
    Name string
}

// NewUser is the constructor pattern — returns concrete type or interface as needed.
func NewUser(id int, name string) (*User, error) {
    if name == "" {
        return nil, fmt.Errorf("NewUser: name must not be empty")
    }
    return &User{ID: id, Name: name}, nil
}

func main() {
    u, err := NewUser(1, "alice")
    if err != nil {
        // log.Fatal / os.Exit only in main; never in library code.
        fmt.Println("error:", err)
        return
    }
    fmt.Println(u.Name)
}
```

## Workflows

### 1. Writing new code

- [ ] Run `gofmt -w .` (or `goimports -w .`) before every commit
- [ ] Lint with `golangci-lint run`; fix all `errcheck`, `staticcheck`, `govet` findings
- [ ] Name exported symbols clearly; unexported helpers use short names (`buf`, `n`, `err`)
- [ ] Every exported function/type has a doc comment starting with its name
- [ ] Keep functions under ~40 lines; extract helpers when a function does more than one thing
- [ ] Use `errors.Is` / `errors.As` for error checks, never string comparison

### 2. Reviewing hot-path functions

Run this mental checklist whenever touching a function called in a loop or concurrently:

- [ ] **`make` placement** — is any `make([]T, ...)` or `make(map[K]V)` inside a loop? If the slice/map is only needed within the loop body and reset each iteration, move it outside and reuse (see §Allocation rules below)
- [ ] **Slice reuse** — prefer `s = s[:0]` over `s = nil` or re-`make`; the backing array is retained
- [ ] **Map reuse** — `for k := range m { delete(m, k) }` clears without reallocation (Go 1.11+: `clear(m)`)
- [ ] **`sync.Pool`** — if the function is called concurrently and allocates a large buffer or struct, pool it
- [ ] **`strings.Builder` / `bytes.Buffer`** — prefer over repeated `+` string concatenation in loops
- [ ] **Benchmark first** — run `go test -bench=. -benchmem` before and after; do not optimize without data

### 3. Concurrency checklist

- [ ] Every goroutine has a defined owner responsible for its lifetime
- [ ] Use `context.Context` for cancellation; pass it as the **first** parameter
- [ ] Channels: set capacity explicitly when the sender must not block (`make(chan T, n)`)
- [ ] Protect shared state with `sync.Mutex` or `sync.RWMutex`; lock as late as possible, unlock as early as possible
- [ ] Run tests with `-race`; fix all races before merging

### 4. Error handling

```go
// Wrap with context — caller can still unwrap with errors.Is / errors.As.
if err != nil {
    return fmt.Errorf("processOrder %d: %w", id, err)
}

// Sentinel errors for expected conditions.
if errors.Is(err, ErrNotFound) {
    // handle gracefully
}

// Custom error type when callers need to inspect fields.
type ValidationError struct {
    Field   string
    Message string
}
func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation: %s %s", e.Field, e.Message)
}
```

## Allocation rules

### make outside the loop

```go
// BAD — allocates a new slice on every iteration.
for _, item := range items {
    buf := make([]byte, 0, 256)
    buf = process(item, buf)
    sink(buf)
}

// GOOD — single allocation; reset with buf[:0] each iteration.
buf := make([]byte, 0, 256)
for _, item := range items {
    buf = buf[:0]          // reset length, keep capacity
    buf = process(item, buf)
    sink(buf)
}
```

### Slice capacity pre-allocation

```go
// When the final length is known up front, set both len and cap.
result := make([]string, 0, len(input))
for _, v := range input {
    result = append(result, transform(v))
}
```

### Map reuse

```go
seen := make(map[string]struct{}, 128) // pre-size to expected load
for _, batch := range batches {
    clear(seen)   // Go 1.21+; or: for k := range seen { delete(seen, k) }
    for _, item := range batch {
        seen[item.Key] = struct{}{}
    }
    processBatch(seen)
}
```

### sync.Pool for concurrent hot paths

```go
var bufPool = sync.Pool{
    New: func() any {
        // Size the buffer for the common case.
        b := make([]byte, 0, 4096)
        return &b
    },
}

func encode(data []byte) []byte {
    bp := bufPool.Get().(*[]byte)
    buf := (*bp)[:0]          // reset, keep backing array

    buf = appendEncoded(buf, data)
    out := make([]byte, len(buf)) // copy out before returning to pool
    copy(out, buf)

    *bp = buf
    bufPool.Put(bp)
    return out
}
```

> **Rule:** pool objects that are ≥ ~1 KB and allocated more than ~100 k/s. For smaller or infrequent allocations the GC overhead of pooling can exceed the savings.

## Advanced features

See [PERFORMANCE.md](PERFORMANCE.md) for:
- Profiling workflow (`pprof`, `go tool trace`)
- Escape analysis (`go build -gcflags='-m'`)
- Benchmarking template
- `unsafe` and `linkname` patterns (with caveats)
- `io.Reader` / `io.Writer` chain optimisation

## Style quick-reference

| Topic | Rule |
|---|---|
| Naming | `camelCase` for all; `ALLCAPS` only for acronyms (`URL`, `HTTP`, `ID`) |
| Receivers | Short, consistent (`u *User`, not `self` or `this`) |
| Package names | Single lowercase word; no underscores or mixedCaps |
| `init()` | Avoid; prefer explicit initialization in `main` or constructors |
| Naked returns | Avoid in functions > 5 lines |
| Struct tags | `json:"snake_case,omitempty"` — match wire format, not Go name |
| Integer types | Prefer `int`/`uint`; use sized types only when ABI / serialization demands |
| `interface{}` / `any` | Use only at genuine boundaries; add type assertions with `ok` check |

## Toolchain reference

```bash
go build ./...          # compile everything
go test ./... -race     # test with race detector
go vet ./...            # static analysis
gofmt -w .              # format in place
goimports -w .          # format + fix imports
golangci-lint run       # comprehensive lint suite
go test -bench=. -benchmem -count=3   # benchmark with allocation stats
go build -gcflags='-m=2' ./pkg/hot/   # escape analysis on hot package
go tool pprof cpu.prof  # interactive profiler
```
