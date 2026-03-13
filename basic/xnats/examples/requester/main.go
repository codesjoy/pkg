package main

import (
	"fmt"

	"github.com/nats-io/nats.go"

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

	sub, err := nc.Subscribe(cfg.Subject, func(msg *nats.Msg) {
		_ = msg.Respond([]byte("pong"))
	})
	if err != nil {
		panic(err)
	}
	defer sub.Drain()

	requester, err := xnats.NewRequester(xnats.RequesterConfig{
		URLs: []string{cfg.URL},
	})
	if err != nil {
		panic(err)
	}
	defer requester.Close()

	resp, err := requester.Request(nil, &publish.Message{
		Subject: cfg.Subject,
		Data:    []byte("ping"),
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("response: %s\n", string(resp.Data))
}
