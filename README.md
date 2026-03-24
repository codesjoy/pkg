# codesjoy/pkg

Reusable Go libraries, utilities, and tools for Go services.

## Overview

The repository is organized by responsibility:

- `basic/`: domain-oriented foundational libraries for business and platform capabilities.
- `utils/`: general-purpose utilities shared across projects and services.
- `tools/`: modules for build-time workflows and engineering productivity.

## Libraries

| Module | Purpose | Docs |
| --- | --- | --- |
| `basic/xgorm` | Enhanced GORM utilities (pagination, transaction context, plugins) | [basic/xgorm/README.md](./basic/xgorm/README.md) |
| `basic/aipsql` | AIP-160 filtering + AIP-132 sorting + seek pagination + SQL planning | [basic/aipsql/README.md](./basic/aipsql/README.md) |
| `basic/snowflake` | Distributed Snowflake ID generation with static/GORM workers | [basic/snowflake/README.md](./basic/snowflake/README.md) |
| `basic/xevent` | Transport-agnostic domain events with typed dispatch plus Kafka and JetStream adapters | [basic/xevent/README.md](./basic/xevent/README.md) |
| `basic/xerror` | Framework-agnostic domain errors with reason + canonical code matching | [basic/xerror/README.md](./basic/xerror/README.md) |
| `basic/xkafka` | Sarama-based Group/Partition consumers plus Sync/Batch/Async producer with middleware chain (logger/retry/trace), shard ordering, and at-least-once semantics | [basic/xkafka/README.md](./basic/xkafka/README.md) |
| `basic/xmongo` | MongoDB Go driver v2 native-style client builder with observability, logging, and transaction helpers | [basic/xmongo/README.md](./basic/xmongo/README.md) |
| `basic/xnats` | NATS and JetStream extension library with publisher/subscriber/request/reply and middleware support | [basic/xnats/README.md](./basic/xnats/README.md) |
| `basic/xredis` | go-redis native-style client builder with middleware wiring for slog and OpenTelemetry | [basic/xredis/README.md](./basic/xredis/README.md) |
| `basic/xjwt` | JWT-oriented key generation (RSA/ECDSA/Ed25519/X25519/JWK) | [basic/xjwt/README.md](./basic/xjwt/README.md) |
| `utils` | Utility module with `base62`, `xcrypto`, `cookie`, `xemail`, `xgo`, `xmap`, `xnet`, `xsync` | [utils/README.md](./utils/README.md) |
| `proto` | Stable shared protobuf option definitions and generated annotation packages | N/A | Imported by business protobufs that use repository-owned proto extensions |

## Tools

| Tool | Purpose | Docs | Typical use |
| --- | --- | --- | --- |
| `tools/protoc-gen-codesjoy-event` | Protoc plugin for generating `xevent.Event` implementations from annotated protobuf messages | [tools/protoc-gen-codesjoy-event/README.md](./tools/protoc-gen-codesjoy-event/README.md) | Generate `EventType` / `EventID` / `PartitionKey` / protobuf payload helpers for domain events |
| `tools/protoc-gen-google-aip` | Protoc plugin for generating Google AIP helpers for resources, field behavior, and method signatures | [tools/protoc-gen-google-aip/README.md](./tools/protoc-gen-google-aip/README.md) | Generate resource-name parsing/formatting and annotation metadata accessors |
| `tools/protoc-gen-codesjoy-reason` | Protoc plugin for generating enum reason helpers (`Reason`/`Domain`/`Code`) | [tools/protoc-gen-codesjoy-reason/README.md](./tools/protoc-gen-codesjoy-reason/README.md) | Generate reason helper methods for proto enums |
| `tools/codesjoy-modelgen` | Introspect MySQL/PostgreSQL schema and generate GORM models + `aipsql.Table` builders | [tools/codesjoy-modelgen/README.md](./tools/codesjoy-modelgen/README.md) | Bootstrap model layer and AIP filter schema from existing tables |

## Quick Start

### For library users

Install only what you need:

```bash
go get github.com/codesjoy/pkg/basic/xgorm
go get github.com/codesjoy/pkg/basic/aipsql
go get github.com/codesjoy/pkg/basic/snowflake
go get github.com/codesjoy/pkg/basic/xevent
go get github.com/codesjoy/pkg/basic/xevent/nats
go get github.com/codesjoy/pkg/basic/xerror
go get github.com/codesjoy/pkg/basic/xkafka
go get github.com/codesjoy/pkg/basic/xmongo
go get github.com/codesjoy/pkg/basic/xnats
go get github.com/codesjoy/pkg/basic/xredis
go get github.com/codesjoy/pkg/basic/xjwt
go get github.com/codesjoy/pkg/utils
```

### For tool users

Install the tools you need:

```bash
go install github.com/codesjoy/pkg/tools/protoc-gen-google-aip@latest
go install github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason@latest
go install github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-event@latest
go install github.com/codesjoy/pkg/tools/codesjoy-modelgen@latest
```

Then follow:
- [Plugin docs](./tools/protoc-gen-codesjoy-event/README.md)
- [Plugin docs](./tools/protoc-gen-codesjoy-reason/README.md)

## Development

Requirements: Go 1.25.7+, Make, Python 3 (for `pre-commit`), Docker (for integration scenarios).

```bash
make tools
make sync
make fmt
make lint
make test
make coverage
make changelog.init
make changelog.verify
make changelog.state.print
```

Common scoped runs:

```bash
make MODULES="utils" lint
make MODULES="basic/xjwt" test
make MODULE_EXCLUDE="basic/snowflake/examples" test
make changelog.preview CHANGELOG_PATHS="basic/xkafka"
make changelog CHANGELOG_PROFILE=high-frequency
```

No-tag repository friendly mode (default `balanced`) supports rolling `unreleased` plus configurable archive cadence:

```bash
make changelog CHANGELOG_PROFILE=balanced
make changelog CHANGELOG_CADENCE=weekly
make changelog.state.reset
```

If changelog scaffolding files are missing, run:

```bash
make changelog.init
```

`make changelog.init` only backfills missing files/directories and does not overwrite existing changelog config/template.

See [DEVELOPMENT.md](./DEVELOPMENT.md) for workflow details and [MAKEFILE_QUICK_REFERENCE.md](./MAKEFILE_QUICK_REFERENCE.md) for target/variable lookup.

## Documentation Index

- [DEVELOPMENT.md](./DEVELOPMENT.md): contributor workflow and local development guide.
- [MAKEFILE_QUICK_REFERENCE.md](./MAKEFILE_QUICK_REFERENCE.md): make targets and variables.
- [CHANGELOG.md](./CHANGELOG.md): release notes generated from Conventional Commits.
- [basic/aipsql/docs/README.md](./basic/aipsql/docs/README.md): deep documentation for `aipsql`.

## Contributing

1. Run `make fmt`.
2. Run `make lint`.
3. Run `make test`.
4. Add or update tests with code changes.
5. Update docs when behavior or APIs change.

## License

Copyright 2022-2025 The codesjoy Authors.

Licensed under the Apache License, Version 2.0.
