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

package partition

import (
	"context"
	"fmt"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/offset"
)

// OffsetStore stores checkpoint next offset per topic/partition.
// 分区 offset 的持久化存储接口。
type OffsetStore interface {
	// Load 加载指定 topic:partition 的下一个待消费 offset。
	Load(
		ctx context.Context,
		topic string,
		partition int32,
	) (nextOffset int64, found bool, err error)
	// Save 持久化指定 topic:partition 的下一个待消费 offset。
	Save(ctx context.Context, topic string, partition int32, nextOffset int64) error
}

// offsetCommitter 管理分区 offset 的跟踪和持久化提交。
type offsetCommitter struct {
	// topic 是目标 topic。
	topic string
	// partition 是目标分区。
	partition int32
	// store 是 offset 持久化存储。
	store OffsetStore
	// tracker 是连续前沿跟踪器。
	tracker *offset.Tracker
}

// newOffsetCommitter 创建 offset 提交器。
func newOffsetCommitter(topic string, partition int32, store OffsetStore) *offsetCommitter {
	return &offsetCommitter{
		topic:     topic,
		partition: partition,
		store:     store,
		tracker:   offset.NewTracker(),
	}
}

// observe 观察（记录）一个已消费的 offset。
func (c *offsetCommitter) observe(offsetValue int64) {
	c.tracker.Observe(offsetValue)
}

// markDone 标记一个 offset 为已完成，若连续前沿推进则持久化保存。
func (c *offsetCommitter) markDone(ctx context.Context, offsetValue int64) error {
	nextOffset, advanced := c.tracker.MarkDone(offsetValue)
	if !advanced {
		return nil
	}

	// 持久化新的前沿 offset
	if err := c.store.Save(ctx, c.topic, c.partition, nextOffset); err != nil {
		return fmt.Errorf("save offset %s:%d=>%d: %w", c.topic, c.partition, nextOffset, err)
	}
	return nil
}
