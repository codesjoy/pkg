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

package router

import (
	"fmt"
	"hash/fnv"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

// ConsumeKeyExtractor derives logical key for consume shard routing.
type ConsumeKeyExtractor func(*sarama.ConsumerMessage) (string, error)

// DefaultConsumeKeyExtractor extracts message key and falls back to topic:partition.
// 默认消费者键提取器：优先使用消息 Key，空值时回退到 topic:partition。
func DefaultConsumeKeyExtractor(msg *sarama.ConsumerMessage) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("consumer message is nil")
	}
	// 优先使用消息 Key
	if len(msg.Key) > 0 {
		return string(msg.Key), nil
	}
	// 回退到 topic:partition 格式
	return ConsumeFallbackKey(msg), nil
}

// ConsumeFallbackKey returns stable consume fallback key using topic and partition.
func ConsumeFallbackKey(msg *sarama.ConsumerMessage) string {
	if msg == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", msg.Topic, msg.Partition)
}

// ProduceDispatchKey picks key used by producer sharded dispatch.
func ProduceDispatchKey(msg *produce.Message) string {
	if msg == nil {
		return ""
	}
	if len(msg.Key) > 0 {
		return string(msg.Key)
	}
	return msg.Topic
}

// ShardForKey hashes key and maps it into [0, shardCount).
// 使用 FNV-1a 哈希算法计算键的分片索引，结果在 [0, shardCount) 范围内。
func ShardForKey(key string, shardCount int) int {
	// FNV-1a 哈希计算
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	// 取模映射到分片范围
	return int(h.Sum32() % uint32(shardCount))
}
