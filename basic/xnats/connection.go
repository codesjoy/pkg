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

package xnats

import (
	"errors"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func connect(urls []string, opts []nats.Option) (*nats.Conn, error) {
	normalized := normalizeStrings(urls)
	if len(normalized) == 0 {
		return nil, errors.New("nats URLs are required")
	}
	return nats.Connect(strings.Join(normalized, ","), opts...)
}

func newJetStream(conn *nats.Conn) (jetstream.JetStream, error) {
	if conn == nil {
		return nil, ErrJetStreamRequired
	}
	return jetstream.New(conn)
}

func drainConnection(conn *nats.Conn) error {
	if conn == nil {
		return nil
	}
	err := conn.Drain()
	conn.Close()
	return err
}

func drainSubscriptions(subs []*nats.Subscription) error {
	var errs []error
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		if err := sub.Drain(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
