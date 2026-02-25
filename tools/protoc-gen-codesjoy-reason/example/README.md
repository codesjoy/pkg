# Example: protoc-gen-codesjoy-reason

This module shows how to annotate a reason enum and generate helper methods
with `protoc-gen-codesjoy-reason`.

## Structure

- `proto/codesjoy/example/user/v1/reason.proto`: sample reason enum.
- `protogen/codesjoy/example/user/v1/reason.pb.go`: `protoc-gen-go` output.
- `protogen/codesjoy/example/user/v1/reason_reason.pb.go`: `protoc-gen-codesjoy-reason` output.
- `main.go`: runnable demo.

## Install Tools

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason@latest
```

For local repository development, you can install the local plugin checkout:

```bash
go install ../
```

Also make sure `buf` is installed:

```bash
buf --version
```

## Regenerate (Buf)

From this directory (`tools/protoc-gen-codesjoy-reason/example`):

```bash
buf dep update
buf generate
```

## Run

```bash
go run .
```

Expected output:

```text
reason=USER_REASON_NOT_FOUND domain=codesjoy.example.user.v1 code=NOT_FOUND
```
