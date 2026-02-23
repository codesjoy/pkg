# Makefile Quick Reference Guide

Fast reference for day-to-day commands in `codesjoy/pkg`.

## Essential Commands

### Setup

```bash
make tools
make sync
make tidy
```

### Daily Development

```bash
make fmt
make lint
make test
make coverage
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
| Dependencies | `make sync` | Rebuild `go.work` from discovered modules |
| Dependencies | `make tidy` | `go mod tidy` for all modules |
| Dependencies | `make download` | Download module dependencies |
| Tooling | `make tools` | Install tools and git hooks |
| Tooling | `make tools.list` | Show tool categories and install status |
| Git hooks | `make githooks.install` | Install hooks into `.git/hooks` |
| Git hooks | `make githooks.verify` | Verify hooks are installed |
| Maintenance | `make clean` | Remove build/test artifacts |
| Maintenance | `make copyright` | Add/update copyright headers |
| Discovery | `make help` / `make help.targets` | Show usage/targets |

## Module Selection Variables

### Priority (highest to lowest)

1. `MODULES` (explicit module list)
2. `MODULE_INCLUDE` / `MODULE_EXCLUDE` (filter discovered modules)
3. `MODULES_DIR` (legacy resolver for `go.*.<module>` shorthand)

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

# Optional exclusion pattern passed to linters
EXCLUDE_TESTS="vendor|test"
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
```

### Coverage gate failing

```bash
make go.test.coverage.all
make coverage COVERAGE=50
```
