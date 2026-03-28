// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/examples/internal/examplecfg"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

func main() {
	cfg, err := examplecfg.Load()
	if err != nil {
		panic(err)
	}

	nc, err := nats.Connect(cfg.URL)
	if err != nil {
		panic(err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		panic(err)
	}
	if _, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     cfg.Stream,
		Subjects: []string{cfg.Subject},
	}); err != nil {
		panic(err)
	}
	if _, err := js.CreateOrUpdateConsumer(context.Background(), cfg.Stream, jetstream.ConsumerConfig{
		Name:          cfg.Consumer,
		FilterSubject: cfg.Subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	}); err != nil {
		panic(err)
	}

	consumer, err := xnats.NewJetStreamConsumer(xnats.JetStreamConsumerConfig{
		Conn:          nc,
		JetStream:     js,
		Stream:        cfg.Stream,
		Consumer:      cfg.Consumer,
		Mode:          xnats.JetStreamConsumerModePull,
		PullBatchSize: 1,
		Logger:        examplecfg.NewLogger(),
	})
	if err != nil {
		panic(err)
	}
	defer consumer.Close()

	ctx, cancel := examplecfg.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	err = consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
		fmt.Printf("pull consumed: %s\n", string(msg.Message.Data))
		cancel()
		return nil
	})
	if err != nil && err != context.Canceled {
		panic(err)
	}
}
