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
	"time"

	"github.com/nats-io/nats.go"
)

const defaultRequestTimeout = 5 * time.Second

// RequesterConfig configures Requester.
type RequesterConfig struct {
	URLs           []string
	Conn           *nats.Conn
	ConnectOptions []nats.Option
	Timeout        time.Duration
}

// Validate normalizes and validates requester config.
func (cfg *RequesterConfig) Validate() error {
	if cfg == nil {
		return errors.New("requester config is nil")
	}
	cfg.URLs = normalizeStrings(cfg.URLs)
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultRequestTimeout
	}
	if len(cfg.URLs) == 0 && cfg.Conn == nil {
		return errors.New("requester URLs are required")
	}
	return nil
}
