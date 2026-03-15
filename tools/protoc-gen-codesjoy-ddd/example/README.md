# Example: protoc-gen-codesjoy-ddd

This module shows how to annotate protobuf event messages and generate
`xevent.Event` helpers with `protoc-gen-codesjoy-ddd`.

The demo sends a generated `OrderCreated` protobuf message through
`xevent.Dispatcher` using the generated `OnOrderCreated(...)` helper, without
writing manual marshal or metadata methods, and also includes an explicit
`event_type` override on `AuditPing`.

## Structure

- `proto/codesjoy/example/order/v1/order.proto`: sample annotated event messages.
- `protogen/codesjoy/example/order/v1/order.pb.go`: `protoc-gen-go` output.
- `protogen/codesjoy/example/order/v1/order_event.pb.go`: `protoc-gen-codesjoy-ddd` output.
- `main.go`: runnable demo.

## Install Tools

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

For local repository development, no extra plugin installation is required
because `buf.gen.yaml` runs the local checkout via `go run ..`.

Also make sure `buf` is installed:

```bash
buf --version
```

## Regenerate (Buf)

From this directory (`tools/protoc-gen-codesjoy-ddd/example`):

```bash
buf generate
```

## Run

```bash
go run .
```

Expected output:

```text
type=codesjoy.example.order.v1.OrderCreated id=evt_1 key=order-1 user=u_1
```
