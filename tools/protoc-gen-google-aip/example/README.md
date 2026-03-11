# Example: protoc-gen-google-aip

This module shows how to annotate resources and request fields and
generate Go helpers with `protoc-gen-google-aip`.

The demo uses generated typed inputs such as `PublisherNameParts` and
`BookNameParts` together with `FillNameFromParts(...)` and
`FillNameWithPatternFromParts(...)`, parses a request `name` by calling
`ParseBookName(req.Name)` and reading typed fields from the parsed result, and
shows parsing and validating a request `parent` by calling
`ParseBookParent(req.Parent)` and `ValidateBookParent(req.Parent)`.

The generated files inline the actual name and parent matching logic. There is
no generated-code runtime dependency.

If a parent pattern can be inferred but the parent resource itself is not part
of the current codegen context, `ParseParent(...)` still parses and validates
the value; the returned typed parent result simply has an empty
`DescriptorType` in that case.

## Structure

- `proto/codesjoy/example/library/v1/publisher.proto`: publisher resource schema.
- `proto/codesjoy/example/library/v1/book.proto`: book resources and service schema.
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
publisher=publishers/pub-1 pattern=publishers/{publisher}/books/{book} name_valid=true book_publisher=pub-1 book_id=book-1 book_archive="" parent_pub_type=library.googleapis.com/Publisher parent_pub_publisher=pub-1 parent_pub_archive="" parent_pub_valid=true parent_archive_type=library.googleapis.com/Archive parent_archive_publisher="" parent_archive_archive=arc-1 parent_archive_valid=true
```
