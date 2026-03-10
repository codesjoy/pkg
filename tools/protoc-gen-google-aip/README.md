# protoc-gen-google-aip

`protoc-gen-google-aip` is a protoc plugin that generates lightweight Go
helpers for common Google AIP annotations:

- `google.api.resource`
- `google.api.resource_definition`
- `google.api.resource_reference`
- `google.api.field_behavior`
- `google.api.method_signature`

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
  --google-aip_opt=paths=source_relative,features=resources,field_behavior \
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
- `(*<Message>).ParseName()`
- `(*<Message>).ValidateName()`
- `(*<Message>).ParseParent(parent string)` when the resource patterns imply one or more registered parent resources
- `(*<Message>).ValidateParent(parent string)` when the resource patterns imply one or more registered parent resources
- `(*<Message>).FillNameWithPattern(pattern string, values map[string]string)`
- `(*<Message>).FillName(values map[string]string)` when the resource has exactly one pattern

For message-level metadata it adds symbol-scoped helpers:

- `(<Message>) GoogleAIPResourceReference(fieldName string)`
- `(<Message>) GoogleAIPFieldBehaviors(fieldName string)`
- `(<Message>) GoogleAIPRequiredFields()`

For service-level method signatures it adds:

- `<Service>GoogleAIPMethodSignatures(methodName string)`

Generated resource helpers depend on:

- `github.com/codesjoy/pkg/tools/protoc-gen-google-aip/runtime/googleaip`

## Example

See [`example/`](./example/README.md) for a complete `buf generate` workflow
covering resources, resource references, field behaviors, and method
signatures.
