# utils

General-purpose utility libraries for Go applications.

## Overview

This module provides five focused utility packages for common Go programming tasks:

- **base62** - Base62 encoding/decoding for int64 values
- **xcrypto** - HMAC-SHA256 cryptographic functions
- **cookie** - HTTP Cookie header parsing and formatting
- **xemail** - Email address syntax validation
- **xgo** - Panic-safe goroutine execution

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

## License

Copyright 2022 The codesjoy Authors.

Licensed under the Apache License, Version 2.0.
