# protoc-gen-codesjoy-reason

`protoc-gen-codesjoy-reason` is a protoc plugin that generates enum helpers for
reason-based error classification.

It works with the extension definitions in:
`proto/codesjoy/reason/v1/reason.proto`.

## Install

```bash
go install github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason@latest
```

## Reason Proto Extensions

Use the shared extension proto:

```proto
import "codesjoy/reason/v1/reason.proto";

enum Reason {
  option (codesjoy.reason.v1.default_reason) = 500;

  REASON_UNSPECIFIED = 0 [(codesjoy.reason.v1.code) = OK];
  USER_NOT_FOUND = 1 [(codesjoy.reason.v1.code) = NOT_FOUND];
}
```

## Generate With protoc

```bash
protoc \
  --go_out=. --go_opt=paths=source_relative \
  --codesjoy-reason_out=. --codesjoy-reason_opt=paths=source_relative \
  your/reason.proto
```

## Generate With Buf

```yaml
version: v2

plugins:
  - local: protoc-gen-go
    out: ./gen
    opt: paths=source_relative

  - local: protoc-gen-codesjoy-reason
    out: ./gen
    opt: paths=source_relative
```

Then run:

```bash
buf generate
```

## Generated API Contract

For enums marked with `(codesjoy.reason.v1.default_reason)`, the plugin generates:

- `<Enum>_code map[int32]code.Code`
- `(r <Enum>) Reason() string`
- `(r <Enum>) Domain() string`
- `(r <Enum>) Code() code.Code`

These methods are directly compatible with
`github.com/codesjoy/pkg/basic/xerror` (`xerror.Reason` interface).

## Testing

```bash
go test ./...
go test -race ./...
```

## Example

See the standalone example module:
[`example/`](./example/README.md), which regenerates code via `buf generate`.
