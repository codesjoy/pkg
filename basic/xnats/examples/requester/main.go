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
