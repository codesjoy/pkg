# xevent/outbox

`xevent/outbox` is the namespace for the two supported outbox styles in
`xevent`:

- `github.com/codesjoy/pkg/basic/xevent/outbox/relay`
- `github.com/codesjoy/pkg/basic/xevent/outbox/debezium`

## Choose A Variant

- `outbox/relay`: application-managed relay semantics with claim ownership,
  retries, delayed delivery, and failure state tracking
- `outbox/debezium`: Kafka-only append-only rows intended for Debezium's
  outbox event router

Both GORM adapter READMEs include PostgreSQL / MySQL notes and reference schema
SQL:

- `outbox/relay/gorm`: stateful relay storage, claim dialect notes, and DDL
- `outbox/debezium/gorm`: append-only storage, retention notes, and DDL

## Migration

This repository no longer exposes the stateful relay implementation at the root
`xevent/outbox` path.

- old `github.com/codesjoy/pkg/basic/xevent/outbox` ->
  `github.com/codesjoy/pkg/basic/xevent/outbox/relay`
- old `github.com/codesjoy/pkg/basic/xevent/outbox/gorm` ->
  `github.com/codesjoy/pkg/basic/xevent/outbox/relay/gorm`
