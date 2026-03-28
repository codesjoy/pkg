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
