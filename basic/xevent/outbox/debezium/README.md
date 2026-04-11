# xevent/outbox/debezium

`xevent/outbox/debezium` provides an append-only outbox model for
[`xevent`](https://pkg.go.dev/github.com/codesjoy/pkg/basic/xevent) events that
are published by Debezium's outbox event router.

This package is intentionally narrower than `xevent/outbox/relay`:

- no relay loop
- no `pending/sending/sent/failed` state machine
- no delayed delivery via `AvailableAt`
- no application-managed retry bookkeeping

The final Kafka topic must be known before the outbox row is inserted.

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xevent/outbox/debezium
```

## What This Package Provides

- `Record`: append-only Debezium outbox row
- `AppendEvent`: encode an `xevent.Event` and append it into a Debezium outbox
  table
- `NewRecord`: convert an `xevent.Outbound` into an append-only outbox row
- `Store`: minimal persistence contract for append-only rows

## Topic Resolution

`AppendEvent` resolves the final Kafka topic in this order:

1. `event.Topic()`
2. `AppendOptions.Topic`
3. error with `ErrTopicRequired`

This package does not support a runtime default topic because Debezium routes
the record based on the table row content.

## GORM Adapter

The optional GORM adapter is available at:

- `github.com/codesjoy/pkg/basic/xevent/outbox/debezium/gorm`

## Relationship To `xevent/outbox/relay`

Use `xevent/outbox/relay` when you need application-managed relay semantics
such as retry tracking, delayed delivery, claim ownership, or failure states.

Use `xevent/outbox/debezium` when Kafka is the only target and Debezium is
responsible for turning append-only outbox rows into Kafka messages.
