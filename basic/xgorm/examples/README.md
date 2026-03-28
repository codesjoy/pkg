# xgorm Examples

This module contains runnable examples for `github.com/codesjoy/pkg/basic/xgorm`
and the callback-first transaction adapter in
`github.com/codesjoy/pkg/basic/transaction/gorm`.

## Prerequisites

- Go 1.25.7+
- Docker Desktop (or another compatible Docker daemon)

## Run PostgreSQL Locally

```bash
docker run --name xgorm-example-db \
  -e POSTGRES_USER=xgorm \
  -e POSTGRES_PASSWORD=xgorm \
  -e POSTGRES_DB=xgorm_example \
  -p 5432:5432 \
  -d postgres:15-alpine
```

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `XGORM_DSN` | `postgres://xgorm:xgorm@127.0.0.1:5432/xgorm_example?sslmode=disable` | PostgreSQL DSN used by the examples |

## Run

From this directory (`basic/xgorm/examples`):

```bash
GOWORK=off go run ./postgres
```

## Notes

- The example demonstrates `WrapPageQuery`, `transaction/gorm.Within`, and `FindOne`.
- The example uses the same PostgreSQL-style setup as the integration tests in `../testing/integration`.
