# Repository Guidelines

## Project Structure & Module Organization
This repository is a multi-module Go workspace managed by [`go.work`](./go.work). Core code is grouped by responsibility:

- `basic/`: reusable platform/business libraries (for example `basic/xgorm`, `basic/xkafka`, `basic/aipsql`), each with its own `go.mod`.
- `utils/`: shared utility module (`base62`, `xcrypto`, `xmap`, `xnet`, etc.).
- `tools/`: developer tools and code generators (for example `tools/codesjoy-modelgen`, `tools/protoc-gen-codesjoy-reason`).
- `scripts/`: Make rule fragments, hooks, and shell utilities.
- `_output/`: generated artifacts such as coverage reports.

Keep tests close to code (`*_test.go`). Integration tests live under module paths like `testing/integration/`.

## Build, Test, and Development Commands
Use root `Makefile` targets:

- `make tools && make hooks.install && make sync`: install tooling/hooks and refresh workspace.
- `make fmt`: run `gofmt`, `gofumpt`, `goimports`, `golines`.
- `make lint`: run configured `golangci-lint` checks.
- `make test`, `make test.race`, `make test.bench`: unit/race/benchmark runs.
- `make coverage`: coverage gate (default threshold `COVERAGE=60`), outputs to `_output/coverage/`.
- `make check.fast` / `make check`: aggregated local/CI-style gates.

Scope commands when possible, for example: `make MODULES="basic/xgorm" test`.

## Coding Style & Naming Conventions
- Target Go version: 1.25.7+.
- Follow idiomatic Go naming: lowercase package names, exported identifiers in `CamelCase` with GoDoc comments.
- Run `make fmt` before committing; generated files are excluded by default (set `INCLUDE_GENERATED=1` when needed).
- Keep module names/path segments explicit (for example `basic/xredis`, `tools/codesjoy-modelgen`).

## Testing Guidelines
- Prefer table-driven unit tests alongside implementation files.
- Use integration suites only when behavior depends on external services (often Docker-backed).
- Run scoped checks during development, then full checks before PR:
  - `make MODULES="utils" test`
  - `make check`

## Commit & Pull Request Guidelines
- Commit messages must follow Conventional Commits (enforced by `gitlint`):
  - Module-scoped changes: `type(scope): subject` (for example, `feat(xgorm): add sharding support`).
  - Repo-level changes without a clear module owner: `type: subject`.
- Allowed `type` values: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
- `subject` must start lowercase, be concise, <= 72 chars, and must not end with `.`.
- Use stable, concise scopes (prefer module names like `xgorm`, `xkafka`, `utils`; use tool scopes like `tools-modelgen` when appropriate).
- Commits using `!` must include a `BREAKING CHANGE:` footer.
- PRs should include: affected modules, test commands run, and docs updates for API/behavior changes.
