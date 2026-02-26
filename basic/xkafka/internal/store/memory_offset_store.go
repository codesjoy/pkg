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
type MemoryOffsetStore struct {
	mu      sync.RWMutex
	offsets map[string]int64
}

// NewMemoryOffsetStore creates an empty in-memory offset store.
func NewMemoryOffsetStore() *MemoryOffsetStore {
	return &MemoryOffsetStore{offsets: make(map[string]int64)}
}

// Load returns last saved next offset.
func (s *MemoryOffsetStore) Load(
	ctx context.Context,
	topic string,
	partition int32,
) (nextOffset int64, found bool, err error) {
	select {
	case <-ctx.Done():
		return 0, false, ctx.Err()
	default:
	}

	s.mu.RLock()
	offset, ok := s.offsets[key(topic, partition)]
	s.mu.RUnlock()
	if !ok {
		return 0, false, nil
	}
	return offset, true, nil
}

// Save persists next offset for topic and partition.
func (s *MemoryOffsetStore) Save(
	ctx context.Context,
	topic string,
	partition int32,
	nextOffset int64,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	s.offsets[key(topic, partition)] = nextOffset
	s.mu.Unlock()
	return nil
}

func key(topic string, partition int32) string {
	return fmt.Sprintf("%s:%d", topic, partition)
}
