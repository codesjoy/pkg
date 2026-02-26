# Repository Guidelines

## Project Structure & Module Organization
- This repo is a Go workspace (`go.work`) with independently versioned modules.
- `basic/` contains domain-facing libraries such as `aipsql`, `xgorm`, `snowflake`, `xerror`, `xjwt`, and `xkafka`.
- `utils/` contains shared helpers (`base62`, `xcrypto`, `cookie`, `xemail`, `xgo`).
- `tools/` hosts build-time tooling modules, including `protoc-gen-codesjoy-reason` and its example module.
- Shared automation lives in `scripts/make-rules/*.mk`; local git checks live in `scripts/hooks/`.
- Keep tests and examples close to code (for example, `basic/aipsql/testing/integration` and `basic/snowflake/examples`).

## Build, Test, and Development Commands
- `make tools`: install/update required developer tools and hooks.
- `make sync && make tidy`: refresh `go.work` and tidy dependencies.
- `make fmt` / `make fmt.check`: apply or verify formatting.
- `make lint` / `make fix`: run `golangci-lint` checks or apply supported auto-fixes.
- `make test`, `make test.race`, `make test.bench`, `make coverage`: run unit, race, benchmark, and coverage gates.
- Scope commands while iterating:
  - `make MODULES="basic/xjwt" test`
  - `make MODULES="utils" lint`
  - `make MODULE_EXCLUDE="basic/snowflake/examples" test`

## Coding Style & Naming Conventions
- Follow idiomatic Go style and `gofmt` output (tabs, standard layout).
- Use short, lowercase package names without underscores.
- Add GoDoc comments for exported identifiers (enforced by `revive`).
- Prefer explicit error handling; do not silently ignore errors.
- Run `make fmt lint` before opening a PR.

## Testing Guidelines
- Place unit tests beside implementation files as `*_test.go`.
- Name tests `TestXxx` and benchmarks `BenchmarkXxx`.
- Put dependency-heavy integration tests under `testing/integration/`.
- At minimum run `make test`; use `make coverage` for broader changes.

## Commit & Pull Request Guidelines
- Use Conventional Commits: `type(scope): description`.
- Common types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
- Keep subject lines <= 72 chars, lowercase after `:`, and no trailing period.
- If using `!`, include a `BREAKING CHANGE:` footer in the commit body.
- PRs should include purpose, key changes, tests run, and linked issues/docs where relevant.
