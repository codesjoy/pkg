package main

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/examples/internal/examplecfg"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
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

	publisher, err := xnats.NewJetStreamPublisher(xnats.JetStreamPublisherConfig{
		Conn:           nc,
		JetStream:      js,
		DefaultSubject: cfg.Subject,
		Logger:         examplecfg.NewLogger(),
	})
	if err != nil {
		panic(err)
	}
	defer publisher.Close()

	result, err := publisher.Publish(context.Background(), &publish.Message{
		Data: []byte("hello from xnats jetstream publisher"),
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("published stream=%s sequence=%d\n", result.Stream, result.Sequence)
}
