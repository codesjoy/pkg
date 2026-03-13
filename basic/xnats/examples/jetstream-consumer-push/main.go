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
	legacyJS, err := nc.JetStream()
	if err != nil {
		panic(err)
	}
	if _, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     cfg.Stream,
		Subjects: []string{cfg.Subject},
	}); err != nil {
		panic(err)
	}
	if _, err := legacyJS.AddConsumer(cfg.Stream, &nats.ConsumerConfig{
		Durable:        cfg.Consumer,
		DeliverSubject: nats.NewInbox(),
		AckPolicy:      nats.AckExplicitPolicy,
	}); err != nil {
		panic(err)
	}

	consumer, err := xnats.NewJetStreamConsumer(xnats.JetStreamConsumerConfig{
		Conn:      nc,
		JetStream: js,
		Stream:    cfg.Stream,
		Consumer:  cfg.Consumer,
		Mode:      xnats.JetStreamConsumerModePush,
		Logger:    examplecfg.NewLogger(),
	})
	if err != nil {
		panic(err)
	}
	defer consumer.Close()

	ctx, cancel := examplecfg.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	err = consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
		fmt.Printf("push consumed: %s\n", string(msg.Message.Data))
		cancel()
		return nil
	})
	if err != nil && err != context.Canceled {
		panic(err)
	}
}
