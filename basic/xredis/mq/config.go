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

package mq

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultPayloadField is the stream field name used to store the message body.
	defaultPayloadField = "payload"
	// defaultHeaderPrefix is prepended to header keys in the stream entry.
	defaultHeaderPrefix = "header:"
	// defaultOrderKeyHeader is the header key that carries the ordering key.
	defaultOrderKeyHeader = "x-order-key"

	defaultBlock                  = time.Second
	defaultCount            int64 = 1
	defaultGroupStartID           = "0"
	defaultAutoClaimMinIdle       = 30 * time.Second
	defaultAutoClaimCount         = int64(100)
	defaultIdleBackoff            = 200 * time.Millisecond
	defaultShardCount             = 1
	defaultShardQueueSize         = 1024
)

// PublisherConfig configures Publisher.
type PublisherConfig struct {
	DefaultStream     string
	PayloadField      string
	HeaderPrefix      string
	OrderKeyHeader    string
	OrderedShardCount int
	ShardStreamPrefix string
}

// Validate validates and normalizes publisher config.
func (cfg *PublisherConfig) Validate() error {
	if cfg == nil {
		return nil
	}

	// Trim whitespace from all string fields.
	cfg.DefaultStream = strings.TrimSpace(cfg.DefaultStream)
	cfg.PayloadField = strings.TrimSpace(cfg.PayloadField)
	cfg.HeaderPrefix = strings.TrimSpace(cfg.HeaderPrefix)
	cfg.OrderKeyHeader = strings.TrimSpace(cfg.OrderKeyHeader)
	cfg.ShardStreamPrefix = strings.TrimSpace(cfg.ShardStreamPrefix)

	// Reject negative shard counts.
	if cfg.OrderedShardCount < 0 {
		return fmt.Errorf("mq ordered shard count must be non-negative")
	}

	// Apply defaults for empty fields.
	if cfg.PayloadField == "" {
		cfg.PayloadField = defaultPayloadField
	}
	if cfg.HeaderPrefix == "" {
		cfg.HeaderPrefix = defaultHeaderPrefix
	}
	if cfg.OrderKeyHeader == "" {
		cfg.OrderKeyHeader = defaultOrderKeyHeader
	}
	// Final sanity check after defaults are applied.
	if cfg.PayloadField == "" {
		return fmt.Errorf("mq payload field is required")
	}
	return nil
}

// ConsumerConfig configures Consumer.
type ConsumerConfig struct {
	Stream            string
	Group             string
	Consumer          string
	Block             time.Duration
	Count             int64
	AutoCreateGroup   bool
	GroupStartID      string
	PayloadField      string
	HeaderPrefix      string
	OrderKeyHeader    string
	ShardCount        int
	ShardQueueSize    int
	OrderedShardCount int
	ShardStreamPrefix string
	OwnedShards       []int
	AutoClaimMinIdle  time.Duration
	AutoClaimCount    int64
	IdleBackoff       time.Duration
}

// Validate validates and normalizes consumer config.
func (cfg *ConsumerConfig) Validate() error {
	if cfg == nil {
		return ErrConsumerStreamRequired
	}

	// Trim whitespace from all string fields.
	cfg.Stream = strings.TrimSpace(cfg.Stream)
	cfg.Group = strings.TrimSpace(cfg.Group)
	cfg.Consumer = strings.TrimSpace(cfg.Consumer)
	cfg.GroupStartID = strings.TrimSpace(cfg.GroupStartID)
	cfg.PayloadField = strings.TrimSpace(cfg.PayloadField)
	cfg.HeaderPrefix = strings.TrimSpace(cfg.HeaderPrefix)
	cfg.OrderKeyHeader = strings.TrimSpace(cfg.OrderKeyHeader)
	cfg.ShardStreamPrefix = strings.TrimSpace(cfg.ShardStreamPrefix)

	// Validate required fields.
	if cfg.Stream == "" {
		return ErrConsumerStreamRequired
	}
	if cfg.Group == "" {
		return ErrConsumerGroupRequired
	}
	if cfg.Consumer == "" {
		return ErrConsumerNameRequired
	}
	// Reject negative numeric fields.
	if cfg.Block < 0 {
		return fmt.Errorf("mq block must be non-negative")
	}
	if cfg.Count < 0 {
		return fmt.Errorf("mq count must be non-negative")
	}
	if cfg.ShardCount < 0 {
		return fmt.Errorf("mq shard count must be non-negative")
	}
	if cfg.ShardQueueSize < 0 {
		return fmt.Errorf("mq shard queue size must be non-negative")
	}
	if cfg.OrderedShardCount < 0 {
		return fmt.Errorf("mq ordered shard count must be non-negative")
	}
	if cfg.AutoClaimMinIdle < 0 {
		return fmt.Errorf("mq auto claim min idle must be non-negative")
	}
	if cfg.AutoClaimCount < 0 {
		return fmt.Errorf("mq auto claim count must be non-negative")
	}
	if cfg.IdleBackoff < 0 {
		return fmt.Errorf("mq idle backoff must be non-negative")
	}

	// Apply defaults for zero-valued fields.
	if cfg.Block == 0 {
		cfg.Block = defaultBlock
	}
	if cfg.Count == 0 {
		cfg.Count = defaultCount
	}
	if cfg.ShardCount == 0 {
		cfg.ShardCount = defaultShardCount
	}
	if cfg.ShardQueueSize == 0 {
		cfg.ShardQueueSize = defaultShardQueueSize
	}
	if !cfg.AutoCreateGroup {
		cfg.AutoCreateGroup = true
	}
	if cfg.GroupStartID == "" {
		cfg.GroupStartID = defaultGroupStartID
	}
	if cfg.PayloadField == "" {
		cfg.PayloadField = defaultPayloadField
	}
	if cfg.HeaderPrefix == "" {
		cfg.HeaderPrefix = defaultHeaderPrefix
	}
	if cfg.OrderKeyHeader == "" {
		cfg.OrderKeyHeader = defaultOrderKeyHeader
	}
	if cfg.AutoClaimMinIdle == 0 {
		cfg.AutoClaimMinIdle = defaultAutoClaimMinIdle
	}
	if cfg.AutoClaimCount == 0 {
		cfg.AutoClaimCount = defaultAutoClaimCount
	}
	if cfg.IdleBackoff == 0 {
		cfg.IdleBackoff = defaultIdleBackoff
	}
	// Deduplicate and sort owned shards if provided.
	if len(cfg.OwnedShards) > 0 {
		cfg.OwnedShards = normalizeOwnedShards(cfg.OwnedShards)
	}

	// Post-default validation: ensure all values are positive.
	if cfg.Count <= 0 {
		return fmt.Errorf("mq count must be > 0, got %d", cfg.Count)
	}
	if cfg.ShardCount <= 0 {
		return fmt.Errorf("mq shard count must be > 0, got %d", cfg.ShardCount)
	}
	if cfg.ShardQueueSize <= 0 {
		return fmt.Errorf("mq shard queue size must be > 0, got %d", cfg.ShardQueueSize)
	}
	if cfg.Block <= 0 {
		return fmt.Errorf("mq block must be > 0, got %s", cfg.Block)
	}
	if cfg.AutoClaimMinIdle <= 0 {
		return fmt.Errorf("mq auto claim min idle must be > 0, got %s", cfg.AutoClaimMinIdle)
	}
	if cfg.AutoClaimCount <= 0 {
		return fmt.Errorf("mq auto claim count must be > 0, got %d", cfg.AutoClaimCount)
	}
	if cfg.IdleBackoff <= 0 {
		return fmt.Errorf("mq idle backoff must be > 0, got %s", cfg.IdleBackoff)
	}
	if cfg.PayloadField == "" {
		return fmt.Errorf("mq payload field is required")
	}
	// Validate owned shard indices against ordered shard count.
	if len(cfg.OwnedShards) > 0 {
		if cfg.OrderedShardCount <= 0 {
			return fmt.Errorf("mq ordered shard count must be > 0 when owned shards are set")
		}
		for _, shard := range cfg.OwnedShards {
			if shard < 0 || shard >= cfg.OrderedShardCount {
				return fmt.Errorf(
					"mq owned shard %d out of range for ordered shard count %d",
					shard,
					cfg.OrderedShardCount,
				)
			}
		}
	}
	return nil
}

// normalizeOwnedShards deduplicates and sorts the owned shard indices.
func normalizeOwnedShards(shards []int) []int {
	seen := make(map[int]struct{}, len(shards))
	normalized := make([]int, 0, len(shards))
	for _, shard := range shards {
		if _, ok := seen[shard]; ok {
			continue
		}
		seen[shard] = struct{}{}
		normalized = append(normalized, shard)
	}
	slices.Sort(normalized)
	return normalized
}

// shardLabel converts a shard index to its string label.
func shardLabel(shard int) string {
	return strconv.Itoa(shard)
}
