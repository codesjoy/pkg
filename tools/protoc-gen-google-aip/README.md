# protoc-gen-google-aip

`protoc-gen-google-aip` is a protoc plugin that generates lightweight Go
helpers for common Google AIP annotations:

- `google.api.resource`
- `google.api.resource_definition`

The generator emits one Go file per input proto:
`<generated_filename_prefix>_google_aip.pb.go`. Message-level resources
generate methods on the Go struct; file-level `resource_definition` entries are
validated during generation but do not produce public runtime helpers in v1.

## Install

```bash
go install github.com/codesjoy/pkg/tools/protoc-gen-google-aip@latest
```

## Generate With protoc

```bash
protoc \
  --go_out=. --go_opt=paths=source_relative \
  --google-aip_out=. --google-aip_opt=paths=source_relative \
  your/service.proto
```

Feature filtering is optional:

```bash
protoc \
  --google-aip_out=. \
  --google-aip_opt=paths=source_relative,features=resources \
  your/service.proto
```

## Generate With Buf

```yaml
version: v2

plugins:
  - local: protoc-gen-go
    out: ./gen
    opt: paths=source_relative

  - local: protoc-gen-google-aip
    out: ./gen
    opt: paths=source_relative
```

Then run:

```bash
buf generate
```

## Generated API

For resource-annotated messages it adds methods onto the generated message type:

- `<Message>NamePattern` for single-pattern resources
- `<Message>NamePattern1..N` for multi-pattern resources
- `Parsed<Message>Name`
- `Parsed<Message>Parent` when the resource patterns imply one or more parent patterns
- `Parse<Message>Name(name string)`
- `Validate<Message>Name(name string)`
- `Parse<Message>Parent(parent string)` when the resource patterns imply one or more parent patterns
- `Validate<Message>Parent(parent string)` when the resource patterns imply one or more parent patterns
- `(*<Message>).ParseName()`
- `(*<Message>).ValidateName()`
- `(*<Message>).ParseParent(parent string)` when the resource patterns imply one or more parent patterns
- `(*<Message>).ValidateParent(parent string)` when the resource patterns imply one or more parent patterns
- `(*<Message>).FillNameWithPattern(pattern string, values map[string]string)`
- `(*<Message>).FillName(values map[string]string)` when the resource has exactly one pattern

When a parent pattern can be inferred but the parent resource type is not
available in the current generation context, `Parse<Message>Parent(...)` and
`(*<Message>).ParseParent(...)` still succeed and return the correct `Pattern`
and typed parent fields; in that case `DescriptorType` is the empty string.

`Parse<Message>Name(...)`, `(*<Message>).ParseName()`,
`Parse<Message>Parent(...)`, and `(*<Message>).ParseParent(...)` all return
generated typed results, so callers can read resource variables as fields
rather than indexing into a map.

The generated code inlines resource name parsing, validation, parent parsing,
and formatting logic. There is no generated-code runtime dependency in v1.

## Example

See [`example/`](./example/README.md) for a complete `buf generate` workflow
covering resource name helpers.
