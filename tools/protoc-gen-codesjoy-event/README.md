# protoc-gen-codesjoy-event

`protoc-gen-codesjoy-event` is a protoc plugin that generates
`github.com/codesjoy/pkg/basic/xevent.Event` implementations for protobuf
messages annotated with `codesjoy.ddd.event.v1` options.

It is designed for protobuf binary payloads and integrates directly with
`xevent.Dispatcher`, generated `On<Message>` helpers, `xevent.On[T]`, and the
Kafka adapters in `basic/xevent`.
The annotation proto package itself is published from
`github.com/codesjoy/pkg/proto/codesjoy/ddd`, so generated business code
depends on the stable proto module rather than this tool module.

## Install

```bash
go install github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-event@latest
```

## Event Proto Extensions

Use the shared extension proto:

```proto
import "codesjoy/ddd/event/v1/event.proto";

message OrderCreated {
  option (codesjoy.ddd.event.v1.event) = {};

  string id = 1 [(codesjoy.ddd.event.v1.event_id) = true];
  string order_id = 2 [(codesjoy.ddd.event.v1.partition_key) = true];
  string user_id = 3;
}

message AuditPing {
  option (codesjoy.ddd.event.v1.event) = {
    event_type: "audit.ping"
  };
}
```

Rules:

- only messages with `option (codesjoy.ddd.event.v1.event) = {}` (or a populated value) generate output
- `EventType()` defaults to the protobuf message full name
- `event.event_type` is optional and overrides the fullname-derived default
- `event_id` and `partition_key` are optional
- each message may declare at most one `event_id` field and one `partition_key` field
- annotated fields must be top-level, non-map, non-repeated, non-oneof `string` fields

If you need a stable business event name that survives protobuf package or
message renames, set `event.event_type` explicitly. Otherwise, renaming the package
or message changes the default `EventType()` value.

## Generate With protoc

```bash
protoc \
  --go_out=. --go_opt=paths=source_relative \
  --codesjoy-event_out=. --codesjoy-event_opt=paths=source_relative \
  your/event.proto
```

## Generate With Buf

```yaml
version: v2

plugins:
  - local: protoc-gen-go
    out: ./gen
    opt: paths=source_relative

  - local: protoc-gen-codesjoy-event
    out: ./gen
    opt: paths=source_relative
```

Then run:

```bash
buf generate
```

## Generated API Contract

For each annotated message the plugin generates:

- `var _ xevent.Event = (*<Message>)(nil)`
- `func (*<Message>) EventType() string`
- `func (x *<Message>) EventID() string`
- `func (x *<Message>) PartitionKey() string`
- `func (x *<Message>) MarshalPayload() ([]byte, error)`
- `func (x *<Message>) UnmarshalPayload([]byte) error`
- `func On<Message>(d *xevent.Dispatcher, handlers ...func(context.Context, *<Message>) error) error`

The generated implementation uses `proto.Marshal` / `proto.Unmarshal` and has
no extra runtime package beyond existing `xevent` and protobuf dependencies.

Example:

```go
dispatcher := xevent.NewDispatcher()

err := orderv1.OnOrderCreated(
	dispatcher,
	func(ctx context.Context, event *orderv1.OrderCreated) error {
		return nil
	},
	func(ctx context.Context, event *orderv1.OrderCreated) error {
		return nil
	},
)
```

## Testing

```bash
go test ./...
go test -race ./...
```

## Example

See the standalone example module:
[`example/`](./example/README.md), which regenerates code via `buf generate`
and dispatches an annotated protobuf message through `xevent`.
