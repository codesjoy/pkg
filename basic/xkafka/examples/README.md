# xkafka Examples

This module contains runnable examples for `github.com/codesjoy/pkg/basic/xkafka`.

## Prerequisites

- Go 1.25.7+
- A reachable Kafka broker (default: `127.0.0.1:9092`)

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `XKAFKA_BROKERS` | `127.0.0.1:9092` | Kafka broker list, comma-separated |
| `XKAFKA_TOPIC` | `xkafka-example` | Topic used by examples |
| `XKAFKA_GROUP_ID` | `xkafka-example-group` | Group ID for group/trace consumer examples |
| `XKAFKA_PARTITION` | `0` | Partition ID for partition consumer example |
| `XKAFKA_TIMEOUT` | `30s` | Example process timeout |

## Run

From this directory (`basic/xkafka/examples`):

```bash
go run ./group
go run ./partition
go run ./producer
go run ./trace
```

## Notes

- `group`: ConsumerGroup mode with basic business handler logging.
- `partition`: Single partition mode with `MemoryOffsetStore` checkpoint log.
- `producer`: Sync + batch + async produce in one run.
- `trace`: Explicit OTel trace middleware on producer and consumer.
