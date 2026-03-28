# transaction/mongo

`transaction/mongo` provides the MongoDB adapter for
`github.com/codesjoy/pkg/basic/transaction`.

Use this module when application code wants callback-first transaction handling
with `REQUIRED` propagation and after-commit hooks on top of the MongoDB Go
driver v2.

For the shared model and semantics, see the
[core transaction guide](../README.md).
For DDD-oriented transaction boundary guidance, see
[DDD Usage Pattern](../README.md#ddd-usage-pattern).

## Installation

```bash
go get github.com/codesjoy/pkg/basic/transaction/mongo
```

## Quick Start

```go
package main

import (
	"context"

	"github.com/codesjoy/pkg/basic/transaction"
	mongotx "github.com/codesjoy/pkg/basic/transaction/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			panic(err)
		}
	}()

	collection := client.Database("app").Collection("widgets")
	runner := mongotx.New(client)

	err = runner.Within(context.Background(), func(ctx context.Context) error {
		if _, err := collection.InsertOne(ctx, bson.D{{Key: "_id", Value: "widget-1"}}); err != nil {
			return err
		}

		return transaction.AfterCommit(ctx, func(hookCtx context.Context) error {
			_ = mongo.SessionFromContext(hookCtx)
			return nil
		})
	})
	if err != nil {
		panic(err)
	}
}
```

## Runner Behavior

- `mongotx.New(client)` creates a runner backed by the provided `*mongo.Client`
- `runner.Within(ctx, fn)` starts a session and executes `fn` through
  `session.WithTransaction(...)` when one is missing
- nested `Within` calls reuse the active transaction only when the incoming
  context already carries both an active MongoDB session and the marker for the
  same client

If the runner or its client is nil and no active transaction exists in the
incoming context, `Within` returns `mongo.ErrClientDisconnected`.

## Default Session / Transaction Options

Use `mongotx.WithDefaultConfig(...)` to set default driver options for every
transaction started by the runner.

```go
runner := mongotx.New(
	client,
	mongotx.WithDefaultConfig(mongotx.Config{
		SessionOptions: []options.Lister[options.SessionOptions]{
			options.Session(),
		},
		TransactionOptions: []options.Lister[options.TransactionOptions]{
			options.Transaction(),
		},
	}),
)
```

The adapter clones the provided option slices when the runner is configured and
again when a transaction starts, so later caller mutations do not affect a
running transaction.

## After-Commit Context

After-commit hooks run only after the outermost transaction commits
successfully.

For the Mongo adapter, hook execution happens outside the transaction session:

- `mongo.SessionFromContext(hookCtx)` is nil
- hook work is not part of the committed transaction

If a hook fails, `Within` returns an error joined with
`transaction.ErrAfterCommitFailed`, but the transaction has already committed.
