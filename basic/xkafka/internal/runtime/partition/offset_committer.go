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
type OffsetStore interface {
	Load(
		ctx context.Context,
		topic string,
		partition int32,
	) (nextOffset int64, found bool, err error)
	Save(ctx context.Context, topic string, partition int32, nextOffset int64) error
}

type offsetCommitter struct {
	topic     string
	partition int32
	store     OffsetStore
	tracker   *offset.Tracker
}

func newOffsetCommitter(topic string, partition int32, store OffsetStore) *offsetCommitter {
	return &offsetCommitter{
		topic:     topic,
		partition: partition,
		store:     store,
		tracker:   offset.NewTracker(),
	}
}

func (c *offsetCommitter) observe(offsetValue int64) {
	c.tracker.Observe(offsetValue)
}

func (c *offsetCommitter) markDone(ctx context.Context, offsetValue int64) error {
	nextOffset, advanced := c.tracker.MarkDone(offsetValue)
	if !advanced {
		return nil
	}

	if err := c.store.Save(ctx, c.topic, c.partition, nextOffset); err != nil {
		return fmt.Errorf("save offset %s:%d=>%d: %w", c.topic, c.partition, nextOffset, err)
	}
	return nil
}
