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

package xredis

import (
	"errors"
	"strings"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrEmptyAddrs indicates redis.UniversalOptions.Addrs is empty.
	ErrEmptyAddrs = errors.New("redis universal options addrs is empty")
	// ErrNilHook indicates one hook is nil.
	ErrNilHook = errors.New("redis hook is nil")
)

// Config contains client construction settings.
type Config struct {
	redis.UniversalOptions
}

// Validate validates and normalizes xredis config.
func (c *Config) Validate() error {
	if c == nil {
		return ErrEmptyAddrs
	}

	normalizedAddrs := normalizeAddrs(c.Addrs)
	if len(normalizedAddrs) == 0 {
		return ErrEmptyAddrs
	}

	c.Addrs = normalizedAddrs
	return nil
}

// normalizeAddrs trims whitespace from each address and removes empty entries.
func normalizeAddrs(addrs []string) []string {
	if len(addrs) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		trimmed := strings.TrimSpace(addr)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}
