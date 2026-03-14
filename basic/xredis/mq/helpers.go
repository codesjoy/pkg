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
	"fmt"
	"hash/fnv"
	"reflect"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

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

func asBytes(value interface{}) []byte {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return []byte(typed)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return []byte(fmt.Sprint(typed))
	}
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func isBusyGroupError(err error) bool {
	return err != nil && len(err.Error()) >= len("BUSYGROUP") && err.Error()[:len("BUSYGROUP")] == "BUSYGROUP"
}

func shardForKey(key string, shardCount int) int {
	if shardCount <= 1 {
		return 0
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(shardCount))
}

func trimSpaceOrEmpty(value string) string {
	return strings.TrimSpace(value)
}

type streamBinding struct {
	BaseStream  string
	ShardStream string
	Shard       int
}

func resolveBaseStream(msg *Message, defaultStream string) string {
	if msg != nil {
		if stream := trimSpaceOrEmpty(msg.Stream); stream != "" {
			return stream
		}
	}
	return trimSpaceOrEmpty(defaultStream)
}

func resolveLogicalKey(headers map[string]string, orderKeyHeader string, fallback string) string {
	if headers != nil {
		if key := trimSpaceOrEmpty(headers[orderKeyHeader]); key != "" {
			return key
		}
	}
	return fallback
}

func shardStreamName(baseStream, shardStreamPrefix string, shard int) string {
	if prefix := trimSpaceOrEmpty(shardStreamPrefix); prefix != "" {
		return prefix + ":" + shardLabel(shard)
	}
	return baseStream + ":shard:" + shardLabel(shard)
}

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

func bindingByStream(bindings []streamBinding) map[string]streamBinding {
	index := make(map[string]streamBinding, len(bindings))
	for _, binding := range bindings {
		index[binding.ShardStream] = binding
	}
	return index
}

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

func messageHeaders(msg *Message) map[string]string {
	if msg == nil {
		return nil
	}
	return msg.Headers
}
