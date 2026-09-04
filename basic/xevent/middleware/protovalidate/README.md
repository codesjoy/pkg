# xevent Protovalidate Middleware

This module provides an optional Buf Protovalidate middleware for
`github.com/codesjoy/pkg/basic/xevent`.

Requirements:

- Go 1.26 or later
- `buf.build/go/protovalidate` v1.4.0
- `google.golang.org/protobuf` v1.36.12

The middleware validates only events that implement `proto.Message`. Other
event implementations pass through unchanged. Validation failures are marked
with `xevent.ErrDiscard`, `ErrValidation`, and the original
`*protovalidate.ValidationError`; a Dispatcher therefore returns `nil` for
the message and allows the transport to commit it without retry or DLQ.

```go
import (
	bufprotovalidate "buf.build/go/protovalidate"
	eventprotovalidate "github.com/codesjoy/pkg/basic/xevent/middleware/protovalidate"
)

middleware, err := eventprotovalidate.New(eventprotovalidate.Config{
	ValidatorOptions: []bufprotovalidate.ValidatorOption{
		bufprotovalidate.WithMessages(&OrderCreated{}),
		bufprotovalidate.WithDisableLazy(),
	},
})
if err != nil {
	panic(err)
}
dispatcher.Use(middleware)
```

`New` initializes one concurrent-safe Protovalidate `Validator` for the
middleware. Omit `ValidatorOptions` to use the default validator configuration.
