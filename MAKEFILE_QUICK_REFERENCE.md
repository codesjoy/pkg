# Makefile Quick Reference Guide

Fast reference for day-to-day commands in `codesjoy/pkg`.

## Essential Commands

### Setup

```bash
make tools
make hooks.install
make sync
make tidy
```

### Daily Development

```bash
make fmt
make lint
make test
make coverage
make check.fast
```

### CI Gates

```bash
make fmt.check
make lint
make test.race
make coverage COVERAGE=80
```

## Root Targets (Recommended)

| Category | Target | Purpose |
| --- | --- | --- |
| Formatting | `make fmt` | Run all formatters |
| Formatting | `make fmt.check` | Fail if formatting is not clean |
| Linting | `make lint` | Run linters across selected modules |
| Linting | `make fix` | Run linters with `--fix` |
| Testing | `make test` | Run tests across selected modules |
| Testing | `make test.race` | Run tests with race detector |
| Testing | `make test.bench` | Run benchmarks |
| Coverage | `make coverage` | Coverage run + quality gate |
| Quality | `make check.fast` | Run `fmt.check` + `lint` + `test` |
| Quality | `make check` | Run full gates (`check.fast` + `coverage` + `go.work.drift`) |
| Dependencies | `make sync` | Rebuild `go.work` from discovered modules |
| Dependencies | `make go.work.drift` | Verify `go.work` matches discovered modules |
| Dependencies | `make tidy` | `go mod tidy` for all modules |
| Dependencies | `make download` | Download module dependencies |
| Diagnostics | `make doctor` | Verify env/tools/hooks/workspace |
| Diagnostics | `make modules.print` | Print discovered/selected module context |
| Scripts | `make scripts.lint` | Run `bash -n` + `shfmt -d` + optional `shellcheck` |
| Changelog | `make changelog.init` | Initialize changelog scaffold files/directories |
| Changelog | `make changelog` | Generate/update `CHANGELOG.md` |
| Changelog | `make changelog.preview` | Print generated changelog to stdout |
| Changelog | `make changelog.verify` | Fail when `CHANGELOG.md` is stale |
| Changelog | `make changelog.state.print` | Show effective profile, state, and resolved query |
| Changelog | `make changelog.state.reset` | Reset baseline state to current `HEAD` |
| Tooling | `make tools` | Install tools and pre-commit hooks |
| Tooling | `make tools.list` | Show tool categories and install status |
| Hooks | `make hooks.install` | Install pre-commit + commit-msg hooks |
| Hooks | `make hooks.verify` | Verify hooks are installed |
| Hooks | `make hooks.run` | Run hooks on staged files |
| Hooks | `make hooks.run-all` | Run hooks on all files |
| Maintenance | `make clean` | Remove build/test artifacts |
| Maintenance | `make copyright` | Add/update copyright headers |
| Discovery | `make help` / `make help.targets` | Show usage/targets |

## Module Selection Variables

### Priority (highest to lowest)

1. `MODULES` (explicit module list)
2. `MODULE_INCLUDE` / `MODULE_EXCLUDE` (filter the selected module list)
3. `INCLUDE_EXAMPLES` (default `0`, excludes `*/example` and `*/examples` unless `MODULES` is explicit)
4. `MODULES_DIR` (legacy resolver for `go.*.<module>` shorthand)

### Common Patterns

```bash
# Explicit list
make MODULES="utils basic/xjwt" test

# Single module (recommended)
make MODULES="basic/xgorm" lint

# Exclude one module from default discovery
make MODULE_EXCLUDE="basic/snowflake/examples" test
```

## Advanced `go.*` Targets

Use these when you need finer control than root aliases.

```bash
# Run one formatter only
make go.fmt.gofumpt
make go.fmt.goimports
make go.fmt.golines

# Single-module shorthand (legacy but supported)
make go.test.xjwt
make go.lint.utils
make go.fix.xgorm

# Coverage internals
make go.test.coverage.all
make go.test.coverage.xjwt
```

Notes:

- `go.*.<module>` shorthand resolves module names directly (`utils`) or via `MODULES_DIR` (default: `basic`).
- Prefer `MODULES="..."` + root targets for new scripts and CI jobs.

## Configuration Variables

```bash
# Logging verbosity: 0=debug, 1=info (default), 2=warn, 3=error
LOG_LEVEL=0

# Coverage threshold (default 60)
COVERAGE=80

# Optional regex used to filter package dirs in lint/fix
EXCLUDE_TESTS="vendor|example"

# Include example modules in lint/fix/test/coverage (default: 0)
INCLUDE_EXAMPLES=1

# Include generated Go files in fmt/lint/fix (default: 0)
INCLUDE_GENERATED=1

# Override critical tool versions for make tools
GOLANGCI_LINT_VERSION=v2.7.2
GOFUMPT_VERSION=v0.9.2
GOIMPORTS_VERSION=v0.42.0
GOLINES_VERSION=v0.13.0

# Override shfmt install version
SHFMT_VERSION=v3.12.0

# Override git-chglog install version
GIT_CHGLOG_VERSION=latest

# scripts.lint / doctor: require shellcheck to exist (default: 0, warn only)
SHELLCHECK_REQUIRED=1

# Changelog range/query controls (tags or commit refs)
CHANGELOG_QUERY="v0.1.0..v0.2.0"
CHANGELOG_FROM=v0.1.0
CHANGELOG_TO=v0.2.0
CHANGELOG_PATHS="basic/xkafka tools/protoc-gen-codesjoy-reason"
CHANGELOG_NEXT_TAG=unreleased

# Changelog managed-mode controls (for no-tag/latest repositories)
CHANGELOG_PROFILE=balanced            # simple|balanced|high-frequency
CHANGELOG_CADENCE=monthly            # monthly|weekly|none (explicit override)
CHANGELOG_USE_BASELINE=1
CHANGELOG_ARCHIVE_ENABLE=1
CHANGELOG_STATE_FILE=.chglog/state.env
CHANGELOG_ARCHIVE_DIR=.chglog/archive
CHANGELOG_NOW=2026-03-01             # test-only time override
CHANGELOG_STRICT_STATE=1             # fail on malformed state
```

## Common Workflows

### Before Commit

```bash
make fmt
make fix
make test
make fmt.check
make lint
```

### Focus on One Module

```bash
make MODULES="basic/aipsql" fmt
make MODULES="basic/aipsql" lint
make MODULES="basic/aipsql" test
```

### Diagnose Environment and Module Selection

```bash
make doctor
make modules.print
make scripts.lint
```

### Generate Changelog

```bash
make changelog.init
make changelog
make changelog.preview
make changelog.verify
make changelog.state.print
make changelog CHANGELOG_PROFILE=high-frequency
make changelog CHANGELOG_CADENCE=weekly
make changelog.preview CHANGELOG_PATHS="basic/xkafka"
```

`make changelog.init` is explicit and idempotent: it only creates missing scaffold files/directories and never overwrites existing config/template files.

### Investigate Coverage

```bash
make go.test.coverage.all
# HTML reports: _output/coverage/*.html
```

## Troubleshooting

### Unknown target

```bash
make help.targets
```

### Tool missing

```bash
make tools.list
make tools
```

### Wrong module selected

```bash
make -n MODULES="utils" test
make -n MODULE_INCLUDE="utils basic/xjwt" lint
make -n INCLUDE_EXAMPLES=1 lint
```

### Coverage gate failing

```bash
make go.test.coverage.all
make coverage COVERAGE=50
```

### Changelog out of date

```bash
make changelog
make changelog.verify
```

### Changelog scaffold missing

```bash
make changelog.init
make changelog
```

### Changelog state malformed

```bash
make changelog.state.print
make changelog.state.reset
```
