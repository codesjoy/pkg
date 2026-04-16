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

## Schema Management

The shared outbox table is migration-first infrastructure. Application runtime,
relay workers, and Debezium connectors must assume the table and indexes
already exist.

- use reviewed SQL migrations, not application-startup schema changes
- PostgreSQL template: [`./examples/shared-postgres.sql`](./examples/shared-postgres.sql)
- MySQL template: [`./examples/shared-mysql.sql`](./examples/shared-mysql.sql)
- PostgreSQL legacy upgrade sample:
  [`./examples/migrate-legacy-postgres.sql`](./examples/migrate-legacy-postgres.sql)
- MySQL legacy upgrade sample:
  [`./examples/migrate-legacy-mysql.sql`](./examples/migrate-legacy-mysql.sql)

## Shared Table Migration

When moving existing deployments to the shared `xevent_outbox_records` schema:

1. Apply the shared-table migration SQL.
2. For legacy Debezium rows from `xevent_debezium_outbox_records`, copy them
   into `xevent_outbox_records` with `mode = 'cdc'` and `message_id = id`.
3. For legacy relay rows already in `xevent_outbox_records`, add and backfill
   the shared columns:
   - set `mode = 'relay'`
   - fill empty `message_id` values
   - add `handoff_from_id`
4. Deploy code that reads and writes the shared schema.
5. Update the Debezium connector to `xevent_outbox_records` and
   `transforms.outbox.table.field.event.id = message_id`.
6. If transferring unsent relay backlog to cdc, stop relay workers, wait at
   least one `ClaimTTL`, then run `debeziumgorm.CutoverRelayBacklog(...)`.

The committed legacy upgrade SQL samples above show one concrete way to perform
the backfill and connector cutover.

## Import Path Migration

This repository no longer exposes the stateful relay implementation at the root
`xevent/outbox` path.

- old `github.com/codesjoy/pkg/basic/xevent/outbox` ->
  `github.com/codesjoy/pkg/basic/xevent/outbox/relay`
- old `github.com/codesjoy/pkg/basic/xevent/outbox/gorm` ->
  `github.com/codesjoy/pkg/basic/xevent/outbox/relay/gorm`
