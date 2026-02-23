# Development Guide

This guide describes how to develop, test, and maintain `codesjoy/pkg`.

## Prerequisites

- Go 1.25.7 or later
- Make
- Docker (needed for integration tests that depend on external services)
- Git

## Initial Setup

```bash
git clone https://github.com/codesjoy/pkg.git
cd pkg

make tools
make sync
make tidy
```

Verify your environment:

```bash
make fmt.check
make lint
make test
```

## Daily Workflow

### 1. Format and Lint

```bash
make fmt
make lint
```

Use auto-fix mode when needed:

```bash
make fix
```

### 2. Run Tests

```bash
make test
make test.race
make test.bench
make coverage
```

### 3. Scope Commands to Specific Modules (Recommended)

```bash
make MODULES="utils" test
make MODULES="basic/aipsql" lint
make MODULES="basic/xgorm" coverage
```

You can also filter default module discovery:

```bash
make MODULE_INCLUDE="utils basic/xjwt" test
make MODULE_EXCLUDE="basic/snowflake/examples" test
```

### 4. Legacy Single-Module Shorthand

Legacy shorthand is still supported for `go.*.<module>` targets:

```bash
make go.test.xjwt
make go.lint.utils
```

`MODULES_DIR` is only used by this shorthand resolver. Prefer `MODULES`, `MODULE_INCLUDE`, and `MODULE_EXCLUDE` for new workflows.

## Build System Notes

The root `Makefile` composes modular rules from `scripts/make-rules/*.mk`.

- Human-facing root targets: `make fmt`, `make lint`, `make test`, `make coverage`, etc.
- Internal/advanced targets: `make go.fmt.gofumpt`, `make go.test.coverage.all`, `make tools.list`, etc.

Full target and variable reference: [MAKEFILE_QUICK_REFERENCE.md](./MAKEFILE_QUICK_REFERENCE.md)

## Project Structure

```text
pkg/
├── basic/                    # Core business libraries (separate modules)
│   ├── xgorm/
│   ├── aipsql/
│   ├── snowflake/
│   └── xjwt/
├── utils/                    # General utilities module
├── scripts/                  # Build scripts and Make rule modules
├── githooks/                 # Git hooks installed by `make tools`
├── Makefile                  # Root orchestration
├── README.md                 # Repository overview
└── DEVELOPMENT.md            # This guide
```

## Testing Strategy

### Unit Tests

- Located next to implementation files (`*_test.go`)
- Run with `make test`

### Integration Tests

- Located under module-level integration directories (for example `basic/aipsql/testing/integration/`)
- Usually require Docker-backed services
- Run either via scoped `make` commands or directly inside module directories

Example:

```bash
make MODULES="basic/aipsql" test
# or
cd basic/aipsql && go test ./testing/integration/...
```

### Benchmarks

- Located in `*_bench_test.go`
- Run with `make test.bench`

### Coverage

```bash
make coverage
make go.test.coverage.all
```

Coverage artifacts are generated under `_output/coverage/`.

## Adding a New Module

1. Create a new module directory, for example `basic/newpackage`.
2. Initialize `go.mod` in the new module.
3. Add tests and README for the module.
4. Run `make sync` to refresh `go.work`.
5. Add module entry to [README.md](./README.md).
6. Validate with scoped checks:

```bash
make MODULES="basic/newpackage" fmt
make MODULES="basic/newpackage" lint
make MODULES="basic/newpackage" test
```

## Documentation Conventions

For module README files, keep this order:

1. Overview
2. Installation
3. Quick Start
4. Core API/Concepts
5. Testing
6. Links to deeper docs (if any)

General conventions:

- Use runnable command snippets.
- Keep links relative and valid.
- Avoid duplicating root Make target details across many files; link to [MAKEFILE_QUICK_REFERENCE.md](./MAKEFILE_QUICK_REFERENCE.md).

## Troubleshooting

### Missing tools

```bash
make tools.list
make tools
```

### Formatting or lint failures

```bash
make fmt
make fix
make lint
```

### Module dependency issues

```bash
make sync
make tidy
```

### Git hooks not installed

```bash
make githooks.verify
make githooks.install
```

### Coverage gate fails

```bash
make go.test.coverage.all
make coverage COVERAGE=50
```

## Additional Resources

- [Go Workspaces](https://go.dev/ref/mod#workspaces)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [MAKEFILE_QUICK_REFERENCE.md](./MAKEFILE_QUICK_REFERENCE.md)
