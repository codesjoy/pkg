package main

import (
	"context"
	"fmt"
	"time"

	"github.com/codesjoy/pkg/basic/xnats"
	"github.com/codesjoy/pkg/basic/xnats/examples/internal/examplecfg"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

func main() {
	cfg, err := examplecfg.Load()
	if err != nil {
		panic(err)
	}

	publisher, err := xnats.NewPublisher(xnats.PublisherConfig{
		URLs:           []string{cfg.URL},
		DefaultSubject: cfg.Subject,
		Logger:         examplecfg.NewLogger(),
	})
	if err != nil {
		panic(err)
	}
	defer publisher.Close()

	result, err := publisher.Publish(context.Background(), &publish.Message{
		Data: []byte("hello from xnats publisher"),
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("published to %s at %s\n", result.Subject, result.Published.Format(timeLayout))
}

const timeLayout = time.RFC3339
