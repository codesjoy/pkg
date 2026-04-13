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

package offset

import "sync"

// Tracker tracks observed/done offsets and advances only contiguous ranges.
// Offset 跟踪器，维护已观察和已完成的 offset，仅推进连续的前沿。
type Tracker struct {
	mu sync.Mutex

	// initialized 标记是否已接收首个 offset。
	initialized bool
	// nextOffset 是下一个预期 offset（连续前沿）。
	nextOffset int64
	// doneOffsets 存储已完成但尚未被前沿越过的 offset。
	doneOffsets map[int64]struct{}
}

// NewTracker creates a tracker for one ordered stream.
func NewTracker() *Tracker {
	return &Tracker{doneOffsets: make(map[int64]struct{})}
}

// Observe records one consumed offset.
// 记录一个已消费的 offset，首次调用时初始化前沿位置。
func (t *Tracker) Observe(offset int64) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// 首次初始化或遇到更小 offset 时更新前沿
	if !t.initialized || offset < t.nextOffset {
		t.nextOffset = offset
		t.initialized = true
	}
}

// MarkDone marks one processed offset and returns next commit/save offset when advanced.
// 标记一个 offset 为已完成，并推进连续前沿。返回 (新前沿, 是否推进)。
func (t *Tracker) MarkDone(offset int64) (int64, bool) {
	if t == nil {
		return 0, false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// 首次调用时初始化
	if !t.initialized {
		t.initialized = true
		t.nextOffset = offset
	}

	// 标记完成
	t.doneOffsets[offset] = struct{}{}

	// 连续前沿推进：只要 nextOffset 存在于 doneOffsets 中就向前推进
	advanced := false
	for {
		if _, ok := t.doneOffsets[t.nextOffset]; !ok {
			break
		}
		delete(t.doneOffsets, t.nextOffset)
		t.nextOffset++
		advanced = true
	}

	if !advanced {
		return 0, false
	}
	return t.nextOffset, true
}
