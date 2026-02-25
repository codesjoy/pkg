# xerror

Framework-agnostic domain error package with canonical `code.Code` and
business reason metadata.

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xerror
```

## Quick Start

```go
package main

import (
	"errors"
	"fmt"

	"github.com/codesjoy/pkg/basic/xerror"
	"google.golang.org/genproto/googleapis/rpc/code"
)

type UserReason int

const (
	ReasonUnknown UserReason = iota
	ReasonUserNotFound
)

func (r UserReason) Reason() string {
	switch r {
	case ReasonUserNotFound:
		return "USER_NOT_FOUND"
	default:
		return "UNKNOWN_REASON"
	}
}

func (r UserReason) Domain() string {
	return "user.v1"
}

func (r UserReason) Code() code.Code {
	switch r {
	case ReasonUserNotFound:
		return code.Code_NOT_FOUND
	default:
		return code.Code_UNKNOWN
	}
}

func main() {
	cause := errors.New("record missing")
	err := xerror.WrapWithReason(
		cause,
		ReasonUserNotFound,
		"user lookup failed",
		map[string]string{"user_id": "42"},
	)

	fmt.Println(xerror.IsCode(err, code.Code_NOT_FOUND)) // true
	fmt.Println(xerror.IsReason(err, ReasonUserNotFound)) // true
	fmt.Println(errors.Is(err, cause)) // true
}
```

## API Overview

- `Reason` interface: define business reason + domain + canonical code mapping.
- `New` / `NewWithReason`: create domain errors.
- `Wrap` / `WrapWithReason`: wrap upstream errors while preserving default `errors.Is/As` chain behavior.
- `IsCode` / `IsReason` / `CodeOf` / `ReasonOf`: perform business classification checks.
- `errors.Is`: use for instance/cause chain matching, not for code/reason semantic matching.

## Testing

```bash
go test ./...
```
