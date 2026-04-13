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

package store

import (
	"context"
	"fmt"
	"sync"
)

// MemoryOffsetStore stores per-topic/partition offsets in process memory.
// 进程内内存 offset 存储，使用读写锁保护并发访问。
type MemoryOffsetStore struct {
	// mu 保护 offsets 字段的读写。
	mu sync.RWMutex
	// offsets 按 "topic:partition" 存储 offset 值。
	offsets map[string]int64
}

// NewMemoryOffsetStore creates an empty in-memory offset store.
func NewMemoryOffsetStore() *MemoryOffsetStore {
	return &MemoryOffsetStore{offsets: make(map[string]int64)}
}

// Load returns last saved next offset.
// 加载指定 topic:partition 的 offset，支持 context 取消。
func (s *MemoryOffsetStore) Load(
	ctx context.Context,
	topic string,
	partition int32,
) (nextOffset int64, found bool, err error) {
	// context 取消检查
	select {
	case <-ctx.Done():
		return 0, false, ctx.Err()
	default:
	}

	// 读锁查找
	s.mu.RLock()
	offset, ok := s.offsets[key(topic, partition)]
	s.mu.RUnlock()
	if !ok {
		return 0, false, nil
	}
	return offset, true, nil
}

// Save persists next offset for topic and partition.
// 保存指定 topic:partition 的 offset，支持 context 取消。
func (s *MemoryOffsetStore) Save(
	ctx context.Context,
	topic string,
	partition int32,
	nextOffset int64,
) error {
	// context 取消检查
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// 写锁保存
	s.mu.Lock()
	s.offsets[key(topic, partition)] = nextOffset
	s.mu.Unlock()
	return nil
}

// key 生成 "topic:partition" 格式的存储键。
func key(topic string, partition int32) string {
	return fmt.Sprintf("%s:%d", topic, partition)
}
