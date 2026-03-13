# xnats examples

These examples assume a local NATS server:

```bash
docker run --rm -p 4222:4222 nats:2.10-alpine -js
```

Then run any example from this directory:

```bash
go run ./publisher
go run ./requester
go run ./jetstream-publisher
go run ./jetstream-consumer-pull
go run ./jetstream-consumer-push
```
