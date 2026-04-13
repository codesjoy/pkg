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
	"context"
	"hash/fnv"
	"reflect"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// normalizeContext returns context.Background() when ctx is nil.
func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// isNilClient checks whether a redis.UniversalClient value is nil,
// handling wrapped pointer/interface types via reflection.
func isNilClient(client redis.UniversalClient) bool {
	if client == nil {
		return true
	}

	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// sleepContext waits for the given delay or until ctx is cancelled.
func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isBusyGroupError checks whether the error indicates a BUSYGROUP response
// from XGROUP CREATE (group already exists).
func isBusyGroupError(err error) bool {
	return err != nil && len(err.Error()) >= len("BUSYGROUP") &&
		err.Error()[:len("BUSYGROUP")] == "BUSYGROUP"
}

// shardForKey maps a key to a shard index using FNV-1a hashing.
func shardForKey(key string, shardCount int) int {
	if shardCount <= 1 {
		return 0
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(shardCount))
}

// trimSpaceOrEmpty trims whitespace from the value.
func trimSpaceOrEmpty(value string) string {
	return strings.TrimSpace(value)
}

// streamBinding associates a logical base stream with its physical shard stream.
type streamBinding struct {
	BaseStream  string
	ShardStream string
	Shard       int
}

// resolveBaseStream returns the message stream if set, otherwise the default.
func resolveBaseStream(msg *Message, defaultStream string) string {
	if msg != nil {
		if stream := trimSpaceOrEmpty(msg.Stream); stream != "" {
			return stream
		}
	}
	return trimSpaceOrEmpty(defaultStream)
}

// resolveLogicalKey returns the ordering key from headers if present,
// otherwise falls back to the provided value.
func resolveLogicalKey(headers map[string]string, orderKeyHeader string, fallback string) string {
	if headers != nil {
		if key := trimSpaceOrEmpty(headers[orderKeyHeader]); key != "" {
			return key
		}
	}
	return fallback
}

// shardStreamName builds the physical shard stream name from a prefix or
// falls back to "<baseStream>:shard:<n>".
func shardStreamName(baseStream, shardStreamPrefix string, shard int) string {
	if prefix := trimSpaceOrEmpty(shardStreamPrefix); prefix != "" {
		return prefix + ":" + shardLabel(shard)
	}
	return baseStream + ":shard:" + shardLabel(shard)
}

// orderedPublishBinding resolves the target stream binding and logical key
// for a publish operation, applying sharding when OrderedShardCount > 0.
func orderedPublishBinding(cfg PublisherConfig, msg *Message) (streamBinding, string, error) {
	baseStream := resolveBaseStream(msg, cfg.DefaultStream)
	if baseStream == "" {
		return streamBinding{}, "", ErrMessageStreamRequired
	}
	if cfg.OrderedShardCount <= 0 {
		return streamBinding{
			BaseStream:  baseStream,
			ShardStream: baseStream,
			Shard:       0,
		}, resolveLogicalKey(messageHeaders(msg), cfg.OrderKeyHeader, baseStream), nil
	}

	logicalKey := resolveLogicalKey(messageHeaders(msg), cfg.OrderKeyHeader, baseStream)
	shard := shardForKey(logicalKey, cfg.OrderedShardCount)
	return streamBinding{
		BaseStream:  baseStream,
		ShardStream: shardStreamName(baseStream, cfg.ShardStreamPrefix, shard),
		Shard:       shard,
	}, logicalKey, nil
}

// consumerBindings builds stream bindings for the consumer: either from
// OwnedShards (ordered mode) or a single binding to the base stream.
func consumerBindings(cfg ConsumerConfig) []streamBinding {
	if len(cfg.OwnedShards) == 0 {
		return []streamBinding{{
			BaseStream:  cfg.Stream,
			ShardStream: cfg.Stream,
			Shard:       0,
		}}
	}

	bindings := make([]streamBinding, 0, len(cfg.OwnedShards))
	for _, shard := range cfg.OwnedShards {
		bindings = append(bindings, streamBinding{
			BaseStream:  cfg.Stream,
			ShardStream: shardStreamName(cfg.Stream, cfg.ShardStreamPrefix, shard),
			Shard:       shard,
		})
	}
	return bindings
}

// bindingByStream indexes bindings by their ShardStream name for fast lookup.
func bindingByStream(bindings []streamBinding) map[string]streamBinding {
	index := make(map[string]streamBinding, len(bindings))
	for _, binding := range bindings {
		index[binding.ShardStream] = binding
	}
	return index
}

// readGroupStreams builds the alternating stream-name / ">" slice expected by
// XREADGROUP for reading new messages across all bound shard streams.
func readGroupStreams(bindings []streamBinding) []string {
	args := make([]string, 0, len(bindings)*2)
	for _, binding := range bindings {
		args = append(args, binding.ShardStream)
	}
	for range bindings {
		args = append(args, ">")
	}
	return args
}

// messageHeaders safely returns the message headers map, or nil if msg is nil.
func messageHeaders(msg *Message) map[string]string {
	if msg == nil {
		return nil
	}
	return msg.Headers
}
