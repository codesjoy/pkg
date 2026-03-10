# Example: protoc-gen-google-aip

This module shows how to annotate resources, request fields, and RPC methods and
generate Go helpers with `protoc-gen-google-aip`.

The demo uses generated pattern constants such as `BookNamePattern1` together
with `FillNameWithPattern(...)`, and shows parsing and validating a request
`parent` by calling `Book.ParseParent(req.Parent)` and
`Book.ValidateParent(req.Parent)`.

## Structure

- `proto/codesjoy/example/library/v1/publisher.proto`: publisher resource schema.
- `proto/codesjoy/example/library/v1/book.proto`: book resources, request metadata, and service schema.
- `protogen/codesjoy/example/library/v1/publisher.pb.go`: `protoc-gen-go` output.
- `protogen/codesjoy/example/library/v1/book.pb.go`: `protoc-gen-go` output.
- `protogen/codesjoy/example/library/v1/publisher_google_aip.pb.go`: `protoc-gen-google-aip` output.
- `protogen/codesjoy/example/library/v1/book_google_aip.pb.go`: `protoc-gen-google-aip` output.
- `main.go`: runnable demo.

## Install Tools

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install github.com/codesjoy/pkg/tools/protoc-gen-google-aip@latest
```

For local repository development, install the local plugin checkout:

```bash
go install ../
```

Also make sure `buf` is installed:

```bash
buf --version
```

## Regenerate

From this directory:

```bash
buf dep update
buf generate
```

## Run

```bash
go run .
```

Expected output resembles:

```text
publisher=publishers/pub-1 pattern=publishers/{publisher}/books/{book} parent_pub_type=library.googleapis.com/Publisher parent_pub_vars=map[publisher:pub-1] parent_pub_valid=true parent_archive_type=library.googleapis.com/Archive parent_archive_vars=map[archive:arc-1] parent_archive_valid=true ref_type=library.googleapis.com/Book required=[name] signatures=[[name view]] book_vars=map[book:book-1 publisher:pub-1]
```
