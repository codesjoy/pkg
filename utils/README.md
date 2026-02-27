# utils

General-purpose utility libraries for Go applications.

## Overview

This module provides eight focused utility packages for common Go programming tasks:

- **base62** - Base62 encoding/decoding for int64 values
- **xcrypto** - HMAC-SHA256 cryptographic functions
- **cookie** - HTTP Cookie header parsing and formatting
- **xemail** - Email address syntax validation
- **xgo** - Panic-safe goroutine execution
- **xmap** - Deep map merge, map normalization, and path-based lookup
- **xnet** - Local network address selection and random TCP listen helpers
- **xsync** - Concurrency primitives (event, unbounded FIFO queue, serializer)

All packages are production-ready with comprehensive tests, benchmarks, and zero external dependencies.

## Installation

```bash
go get github.com/codesjoy/pkg/utils
```

## Packages

### base62 - Base62 Encoding

Encode and decode int64 values to/from base62 strings using the alphabet `abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789`.

**Use Cases:**
- URL-safe ID generation
- Short URL services
- ID obfuscation
- Compact numeric representation

**Example:**

```go
import "github.com/codesjoy/pkg/utils/base62"

// Encode an integer to base62
encoded, err := base62.EncodeInt64(123456789)
// encoded = "iwaUH"

// Decode back to integer
decoded, err := base62.DecodeInt64("iwaUH")
// decoded = 123456789

// Handles full int64 range
maxEncoded, _ := base62.EncodeInt64(math.MaxInt64)
// maxEncoded = "k9viXaIfiWh"
```

**API:**
- `EncodeInt64(n int64) (string, error)` - Convert int64 to base62 string
- `DecodeInt64(s string) (int64, error)` - Convert base62 string to int64

**Errors:**
- `ErrNegativeNumber` - Negative numbers are not supported
- `ErrEmptyString` - Cannot decode empty string
- `ErrInvalidCharacter` - String contains non-base62 characters
- `ErrOverflow` - Decoded value exceeds int64 range

---

### xcrypto - Cryptographic Functions

Compute HMAC-SHA256 hashes for message authentication and data integrity verification.

**Use Cases:**
- API signature verification
- Token generation and validation
- Data integrity checks
- Secure message authentication

**Example:**

```go
import (
    "crypto/hmac"
    "github.com/codesjoy/pkg/utils/xcrypto"
)

key := []byte("32-byte-secret-key-1234567890abcd")
data := []byte("message to authenticate")

// Compute HMAC-SHA256
mac := xcrypto.HMACSHA256(key, data)
// mac is 32 bytes

// Verify later (timing-safe comparison)
expectedMAC := xcrypto.HMACSHA256(key, data)
if hmac.Equal(mac, expectedMAC) {
    // Valid - data integrity confirmed
}
```

**API:**
- `HMACSHA256(key []byte, data []byte) []byte` - Compute HMAC-SHA256 hash

**Security Best Practices:**
- Use cryptographically secure random key generators (crypto/rand)
- Key length should be at least 32 bytes (256 bits)
- Never hardcode keys in source code
- Always use `hmac.Equal()` for timing-safe verification
- Rotate keys periodically in production

---

### cookie - HTTP Cookie Utilities

Parse and format HTTP Cookie headers with proper error handling.

**Use Cases:**
- Web servers and middleware
- Proxy development
- Cookie extraction and validation
- HTTP header manipulation

**Example:**

```go
import "github.com/codesjoy/pkg/utils/cookie"

// Extract a specific cookie from raw Cookie header
rawHeader := "session=abc123; user=john; theme=dark"
c, err := cookie.GetCookie(rawHeader, "session")
if err != nil {
    // Handle malformed cookie
}
// c.Name = "session", c.Value = "abc123"

// Parse all cookies
cookies := cookie.Parse(rawHeader)
// Returns []*http.Cookie{{Name: "session", Value: "abc123"}, ...}

// Format cookies back to strings
formatted := cookie.Format(cookies)
// Returns []string{"session=abc123", "user=john", "theme=dark"}
```

**API:**
- `GetCookie(rawCookies, name string) (*http.Cookie, error)` - Extract cookie by name
- `Parse(rawCookies string) []*http.Cookie` - Parse all cookies from header
- `Format(cookies []*http.Cookie) []string` - Convert cookies to string format

**Behavior:**
- `GetCookie` returns `nil, nil` when cookie is not found
- `GetCookie` returns error when header contains malformed fragments
- `Parse` skips invalid fragments silently (does not expose errors)

---

### xemail - Email Validation

Validate email address syntax using RFC-compliant pattern matching.

**Use Cases:**
- Form validation
- User registration
- Email input verification
- Data cleaning

**Example:**

```go
import "github.com/codesjoy/pkg/utils/xemail"

// Validate email addresses
xemail.IsValidEmail("user@example.com")        // true
xemail.IsValidEmail("user.name@example.co.uk") // true
xemail.IsValidEmail("invalid")                  // false
xemail.IsValidEmail("user@")                    // false
xemail.IsValidEmail("@example.com")             // false
```

**API:**
- `IsValidEmail(email string) bool` - Check if email has valid syntax

**Note:** This validates syntax only (ASCII-focused), not mailbox deliverability. Use SMTP verification for deliverability checks.

---

### xgo - Panic-Safe Goroutines

Execute goroutines with automatic panic recovery and logging.

**Use Cases:**
- Background workers
- Concurrent task processing
- Long-running services
- Panic-safe async operations

**Example:**

```go
import (
    "context"
    "log/slog"
    "github.com/codesjoy/pkg/utils/xgo"
)

// Simple panic-safe goroutine
xgo.Go(func() {
    // This code runs in a goroutine
    // Panics are caught and logged automatically
})

// With context propagation
ctx := context.Background()
xgo.GoWithCtx(ctx, func(ctx context.Context) {
    // Context is available here
    // Panics are caught and logged with context
})

// Custom panic handler
runner := xgo.New(
    xgo.WithLogger(slog.Default()),
    xgo.WithPanicHandler(func(info xgo.PanicInfo) {
        // Custom panic handling
        // info.Recovered - panic value
        // info.Stack - stack trace
        // info.Ctx - context if available
    }),
)

runner.Go(func() {
    // Custom panic handling applies
})
```

**API:**
- `Go(f func())` - Run function in goroutine with panic recovery
- `GoWithCtx(ctx context.Context, f func(context.Context))` - Run with context
- `New(opts ...Option) *Runner` - Create custom runner with options

**Options:**
- `WithLogger(logger *slog.Logger)` - Set panic logger (nil to disable)
- `WithPanicHandler(handler PanicHandler)` - Set custom panic callback

**Default Behavior:**
- Panics are logged using `log/slog`
- Stack traces are captured and logged
- Context values are preserved in panic info

---

### xmap - Map Utilities

Operate on dynamic map structures with deep merge, recursive key normalization, and path lookup.

**Use Cases:**
- Merge layered configuration maps (defaults + user + env)
- Normalize decoded YAML-like structures (`map[interface{}]interface{}`) to string-key maps
- Read nested fields from untyped payloads

**Example:**

```go
import "github.com/codesjoy/pkg/utils/xmap"

defaults := map[string]interface{}{
    "server": map[string]interface{}{"host": "0.0.0.0", "port": 8080},
}
override := map[string]interface{}{
    "server": map[string]interface{}{"port": 9000},
}

xmap.MergeStringMap(defaults, override)
// defaults["server"].(map[string]interface{})["port"] == 9000

raw := map[string]interface{}{
    "config": map[interface{}]interface{}{
        "region": "us-east-1",
    },
}
xmap.CoverInterfaceMapToStringMap(raw)
// raw["config"] is map[string]interface{}{"region": "us-east-1"}

value := xmap.DeepSearchInMap(raw, "config", "region")
// value == "us-east-1"
```

**API:**
- `MergeStringMap(dst map[string]interface{}, src ...map[string]interface{})` - Recursively merge source maps into destination
- `ToMapStringInterface(src map[interface{}]interface{}) map[string]interface{}` - Convert interface-key map to string-key map
- `CoverInterfaceMapToStringMap(src map[string]interface{})` - Recursively normalize nested map/slice structures
- `DeepSearchInMap(m map[string]interface{}, paths ...string) interface{}` - Query nested value by path

**Behavior:**
- Type mismatch in `MergeStringMap` is ignored (source value is not applied)
- `DeepSearchInMap` returns `nil` when path is missing or intermediate node is not a map

---

### xnet - Network Utilities

Provide policy-oriented helpers for local address discovery and safe random TCP listening.

**Use Cases:**
- Pick a preferred local IP address (private-first or public-first)
- Enumerate local IPv4/IPv6 addresses with stable ordering
- Bind random TCP ports without TOCTOU race between lookup and listen

**Example:**

```go
import (
    "context"
    "log"
    "github.com/codesjoy/pkg/utils/xnet"
)

ctx := context.Background()

// Select one local address (default policy: private-first, includes CGNAT)
selected, err := xnet.SelectLocalAddr(ctx, xnet.LocalAddrOptions{
    Family: xnet.FamilyAny,
})
if err != nil {
    log.Fatal(err)
}
log.Printf("selected local ip: %s", selected)

// List local addresses (stable, deduplicated, sorted)
all, err := xnet.ListLocalAddrs(ctx, xnet.LocalAddrOptions{
    IncludeLoopback:  true,
    IncludeLinkLocal: true,
})
if err != nil {
    log.Fatal(err)
}
log.Printf("local addrs: %v", all)

// Listen on a random tcp4 port safely and keep listener open
ln, addrPort, err := xnet.ListenTCPRandom(ctx, xnet.ListenOptions{Network: "tcp4"})
if err != nil {
    log.Fatal(err)
}
defer ln.Close()
log.Printf("listening on %s", addrPort)
```

**API:**
- `ListLocalAddrs(ctx context.Context, opts LocalAddrOptions) ([]netip.Addr, error)` - List filtered, deduplicated, sorted local addresses
- `SelectLocalAddr(ctx context.Context, opts LocalAddrOptions) (netip.Addr, error)` - Select one local address by policy
- `ListenTCPRandom(ctx context.Context, opts ListenOptions) (net.Listener, netip.AddrPort, error)` - Listen on random TCP port and return open listener

**Types:**
- `Family` / `FamilyAny` / `FamilyIPv4` / `FamilyIPv6` - Address family controls
- `LocalAddrOptions` - Selection/filter behavior (public/private preference, CGNAT, loopback, link-local)
- `ListenOptions` - Network/host controls for random TCP listen

**Errors:**
- `ErrNoUsableAddress` - No address matches the requested selection policy

---

### xsync - Concurrency Primitives

Provide reusable concurrency primitives built on top of the standard library.

**Use Cases:**
- One-time event signaling across goroutines
- Unbounded FIFO queue for producer/consumer workflows
- Serial callback execution with panic containment and graceful shutdown

**Example:**

```go
import (
    "context"
    "log"
    "github.com/codesjoy/pkg/utils/xsync"
)

ctx := context.Background()

// Event
evt := xsync.NewEvent()
go func() {
    // do work...
    evt.Fire()
}()
if err := evt.Wait(ctx); err != nil {
    log.Fatal(err)
}

// Unbounded queue
q := xsync.NewUnbounded[int]()
_ = q.Put(1)
_ = q.Put(2)
q.Close()
for {
    v, ok := q.Get()
    if !ok {
        break
    }
    log.Printf("queue value=%d", v)
}

// Serializer
s := xsync.NewSerializer(ctx)
defer s.Close()
_ = s.Submit(func(context.Context) { log.Println("task 1") })
_ = s.Submit(func(context.Context) { log.Println("task 2") })
```

**API:**
- `NewEvent() *Event`
- `(*Event).Fire() bool`
- `(*Event).Done() <-chan struct{}`
- `(*Event).HasFired() bool`
- `(*Event).Wait(ctx context.Context) error`
- `NewUnbounded[T any]() *Unbounded[T]`
- `(*Unbounded[T]).Put(v T) error`
- `(*Unbounded[T]).Get() (T, bool)`
- `(*Unbounded[T]).TryGet() (T, bool)`
- `(*Unbounded[T]).Close()`
- `(*Unbounded[T]).Len() int`
- `NewSerializer(ctx context.Context) *Serializer`
- `(*Serializer).Submit(fn func(context.Context)) error`
- `(*Serializer).Close()`
- `(*Serializer).Done() <-chan struct{}`

**Errors:**
- `ErrQueueClosed` - Queue is closed and cannot accept new items
- `ErrSerializerClosed` - Serializer is closed or unavailable

---

## Testing

Run tests for all packages:

```bash
# Run all tests
cd utils && go test ./...

# Run with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. -benchmem ./...

# Race detection
go test -race ./...
```

Run tests for a specific package:

```bash
# Test single package
go test ./base62
go test ./xcrypto
go test ./cookie
go test ./xemail
go test ./xgo
go test ./xmap
go test ./xnet
go test ./xsync
```

## Module Structure

```
utils/
├── base62/           # Base62 encoding/decoding
│   ├── base62.go
│   └── base62_test.go
├── xcrypto/          # Cryptographic functions
│   ├── crypto.go
│   └── crypto_test.go
├── cookie/           # HTTP cookie utilities
│   ├── cookie.go
│   └── cookie_test.go
├── xemail/           # Email validation
│   ├── email.go
│   └── email_test.go
├── xgo/              # Panic-safe goroutines
│   ├── goroutine.go
│   └── goroutine_test.go
├── xmap/             # Map utilities
│   ├── xmap.go
│   └── xmap_test.go
├── xnet/             # Network utilities
│   ├── net.go
│   └── net_test.go
├── xsync/            # Concurrency primitives
│   ├── callback_serializer.go
│   ├── callback_serializer_test.go
│   ├── event.go
│   ├── event_test.go
│   ├── unbounded.go
│   └── unbounded_test.go
├── go.mod
└── README.md
```

## Dependencies

- **Go Version**: 1.25+
- **External Dependencies**: None (standard library only)

## Performance

All packages are optimized for performance:

- **base62**: Efficient encoding/decoding with precomputed lookup tables
- **xcrypto**: Direct crypto/hmac and crypto/sha256 usage
- **cookie**: Zero-allocation parsing where possible
- **xemail**: Precompiled regex pattern
- **xgo**: Minimal overhead panic recovery
- **xmap**: Recursive merge and map normalization helpers
- **xnet**: netip-based filtering and safe random listen helpers
- **xsync**: Reusable event/queue/serializer concurrency primitives

Benchmark results on typical hardware:

```
BenchmarkEncodeInt64-8     100000000    10.2 ns/op
BenchmarkDecodeInt64-8     100000000    12.5 ns/op
BenchmarkHMACSHA256-8       5000000    285 ns/op
BenchmarkGetCookie_Hit-8   30000000     45.2 ns/op
BenchmarkParse-8           10000000    156 ns/op
```

## Best Practices

1. **base62**
   - Always check for errors when encoding/decoding
   - Use for URL-safe IDs, not for security (encoding is reversible)

2. **xcrypto**
   - Use timing-safe comparison (`hmac.Equal`) for verification
   - Generate keys with `crypto/rand`, never hardcode
   - Minimum 32-byte keys for HMAC-SHA256

3. **cookie**
   - Handle `ErrInvalidCookieHeader` when using `GetCookie`
   - Use `Parse` for lenient parsing, `GetCookie` for strict validation

4. **xemail**
   - Validates syntax only, not deliverability
   - For production, combine with SMTP verification

5. **xgo**
   - Always use panic-safe goroutines in production
   - Set up custom panic handlers for critical services
   - Preserve context for tracing and cancellation

6. **xmap**
   - Normalize external/untyped map payloads before deep access
   - Use `MergeStringMap` for layered config merge with deterministic type behavior
   - Use `DeepSearchInMap` for optional nested field reads

7. **xnet**
   - Prefer `SelectLocalAddr` over ad-hoc interface scans in app code
   - Keep default private-first policy unless explicit public-first requirement exists
   - Keep the returned listener from `ListenTCPRandom` open to avoid port races

8. **xsync**
   - Use `Event` for one-time broadcast signaling between goroutines
   - Use `Unbounded[T]` when producer throughput must not block on channel capacity
   - Use `Serializer` to run callbacks in FIFO order and isolate callback panics

## License

Copyright 2022 The codesjoy Authors.

Licensed under the Apache License, Version 2.0.
