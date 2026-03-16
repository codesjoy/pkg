# Development Guide

This guide describes how to develop, test, and maintain `codesjoy/pkg`.

## Prerequisites

- Go 1.25.7 or later
- Make
- Python 3 (for `pre-commit` hook management)
- Docker (needed for integration tests that depend on external services)
- Git

## Initial Setup

```bash
git clone https://github.com/codesjoy/pkg.git
cd pkg

make tools
make hooks.install
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

By default, generated Go files are skipped by `fmt`, `fmt.check`, `lint`, and `fix`. Use `INCLUDE_GENERATED=1` when you need generated outputs included in checks.

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
make check.fast
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
make INCLUDE_EXAMPLES=1 lint
```

### 4. Legacy Single-Module Shorthand

Legacy shorthand is still supported for `go.*.<module>` targets:

```bash
make go.test.xjwt
make go.lint.utils
```

`MODULES_DIR` is only used by this shorthand resolver. Prefer `MODULES`, `MODULE_INCLUDE`, and `MODULE_EXCLUDE` for new workflows.

### 5. Workspace Drift Checks

Most module-scoped `make` commands run with `GOWORK=off`, so day-to-day lint/test commands are not blocked by temporary `go.work` drift.

Use strict checks when you need workspace consistency:

```bash
make go.work.drift
make sync
```

For cross-module development inside this repository, rely on `go.work` to wire local modules together.
Published modules should not carry local `replace` directives in their `go.mod`, because external consumers must be able to resolve versioned dependencies without local filesystem paths.
Example modules may keep local `replace` directives when they are only intended for in-repo development and demonstration.

### 6. Daily Diagnostics and Fast Troubleshooting

Use `doctor` to quickly verify local environment consistency, and `modules.print` to inspect module filtering decisions:

```bash
make doctor
make modules.print
```

Use `scripts.lint` to lint shell scripts (`bash -n` + `shfmt -d`; `shellcheck` optional by default):

```bash
make scripts.lint
make scripts.lint SHELLCHECK_REQUIRED=1
```

### 7. Aggregated Quality Gates

```bash
make check.fast   # fmt.check + lint + test
make check        # check.fast + coverage + go.work.drift
```

### 8. Changelog Management

Generate or verify changelog content from Conventional Commit history:

```bash
make changelog.init
make changelog
make changelog.preview
make changelog.verify
make changelog.state.print
make changelog.state.reset
```

Initialization is explicit by design. `make changelog.init` only creates missing files/directories and never overwrites existing config/template content.

Managed mode (no `CHANGELOG_QUERY/FROM/TO`) supports profile + cadence configuration for long-lived no-tag repositories:

```bash
make changelog CHANGELOG_PROFILE=balanced
make changelog CHANGELOG_PROFILE=high-frequency
make changelog CHANGELOG_CADENCE=weekly
make changelog.preview CHANGELOG_NOW=2026-03-01
```

Manual query mode (explicit range/query, supports tag query or commit refs fallback in no-tag repos):

```bash
make changelog CHANGELOG_FROM=v0.1.0 CHANGELOG_TO=v0.2.0
make changelog.preview CHANGELOG_PATHS="basic/xkafka"
make changelog CHANGELOG_QUERY="v0.1.0..v0.2.0"
make changelog CHANGELOG_QUERY="$(git rev-parse HEAD~20)..$(git rev-parse HEAD)"
```

State file fields (`.chglog/state.env`): `BASE_SHA`, `LAST_SHA`, `CURRENT_BUCKET`.
Use `CHANGELOG_STRICT_STATE=1` to fail on malformed state instead of auto-reset.

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
├── scripts/hooks/            # Local pre-commit helper hooks
├── .pre-commit-config.yaml   # Hook orchestration entry
├── .gitlint                  # Commit message policy
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
make check.fast
```

### Module dependency issues

```bash
make go.work.drift
make sync
make tidy
```

### Changelog verification fails

```bash
make changelog
make changelog.verify
```

### Changelog scaffold missing

```bash
make changelog.init
make changelog
```

### Changelog state malformed or needs baseline reset

```bash
make changelog.state.print
make changelog.state.reset
```

### Pre-commit hooks not installed

```bash
make hooks.verify
make hooks.install
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
