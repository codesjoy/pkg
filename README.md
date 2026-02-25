# codesjoy/pkg

Production-ready Go libraries for building data and platform services.

## Overview

This repository contains multiple Go modules grouped by responsibility:

- `basic/`: domain-facing libraries for database access, query construction, ID generation, and JWT key tooling.
- `utils/`: general-purpose utilities for encoding, crypto helpers, cookie parsing, email validation, and goroutine safety.

Each module is versioned and consumed independently.

## Package Catalog

| Module | Description | Primary Docs |
| --- | --- | --- |
| `basic/xgorm` | Enhanced GORM utilities (pagination, transaction context, plugins) | [basic/xgorm/README.md](./basic/xgorm/README.md) |
| `basic/aipsql` | AIP-160 filtering + AIP-132 sorting + seek pagination + SQL planning | [basic/aipsql/README.md](./basic/aipsql/README.md) |
| `basic/snowflake` | Distributed Snowflake ID generation with static/GORM workers | [basic/snowflake/README.md](./basic/snowflake/README.md) |
| `basic/xerror` | Framework-agnostic domain errors with reason + canonical code matching | [basic/xerror/README.md](./basic/xerror/README.md) |
| `basic/xjwt` | JWT-oriented key generation (RSA/ECDSA/Ed25519/X25519/JWK) | [basic/xjwt/README.md](./basic/xjwt/README.md) |
| `utils` | Utility module with `base62`, `xcrypto`, `cookie`, `xemail`, `xgo` | [utils/README.md](./utils/README.md) |

## Quick Start

Install only what you need:

```bash
go get github.com/codesjoy/pkg/basic/xgorm
go get github.com/codesjoy/pkg/basic/aipsql
go get github.com/codesjoy/pkg/basic/snowflake
go get github.com/codesjoy/pkg/basic/xerror
go get github.com/codesjoy/pkg/basic/xjwt
go get github.com/codesjoy/pkg/utils
```

## Development Entry

Requirements: Go 1.25.7+, Make, Python 3 (for `pre-commit`), Docker (for integration scenarios).

```bash
make tools
make sync
make fmt
make lint
make test
make coverage
```

Common scoped runs:

```bash
make MODULES="utils" lint
make MODULES="basic/xjwt" test
make MODULE_EXCLUDE="basic/snowflake/examples" test
```

See [DEVELOPMENT.md](./DEVELOPMENT.md) for workflow details and [MAKEFILE_QUICK_REFERENCE.md](./MAKEFILE_QUICK_REFERENCE.md) for target/variable lookup.

## Documentation Map

| Audience | Start Here | Then |
| --- | --- | --- |
| Library users | This file | Module README in the catalog above |
| `aipsql` users | [basic/aipsql/README.md](./basic/aipsql/README.md) | [basic/aipsql/docs/README.md](./basic/aipsql/docs/README.md) |
| Contributors | [DEVELOPMENT.md](./DEVELOPMENT.md) | [MAKEFILE_QUICK_REFERENCE.md](./MAKEFILE_QUICK_REFERENCE.md) |
| Maintainers | [MAKEFILE_QUICK_REFERENCE.md](./MAKEFILE_QUICK_REFERENCE.md) | `make help.targets` |

## Contributing

1. Run `make fmt`.
2. Run `make lint`.
3. Run `make test`.
4. Add or update tests with code changes.
5. Update docs when behavior or APIs change.

## License

Copyright 2022-2025 The codesjoy Authors.

Licensed under the Apache License, Version 2.0.
