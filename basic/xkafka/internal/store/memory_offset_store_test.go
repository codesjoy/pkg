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
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryOffsetStoreLoadSave(t *testing.T) {
	t.Parallel()

	s := NewMemoryOffsetStore()
	offset, found, err := s.Load(context.Background(), "orders", 1)
	require.NoError(t, err)
	require.False(t, found)
	require.Equal(t, int64(0), offset)

	require.NoError(t, s.Save(context.Background(), "orders", 1, 42))
	offset, found, err = s.Load(context.Background(), "orders", 1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(42), offset)
}

func TestMemoryOffsetStoreConcurrent(t *testing.T) {
	t.Parallel()

	s := NewMemoryOffsetStore()
	const workers = 16

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(offset int64) {
			defer wg.Done()
			require.NoError(t, s.Save(context.Background(), "orders", 0, offset))
		}(int64(i + 1))
	}
	wg.Wait()

	offset, found, err := s.Load(context.Background(), "orders", 0)
	require.NoError(t, err)
	require.True(t, found)
	require.GreaterOrEqual(t, offset, int64(1))
	require.LessOrEqual(t, offset, int64(workers))
}

func TestMemoryOffsetStoreContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := NewMemoryOffsetStore()
	_, _, err := s.Load(ctx, "orders", 0)
	require.ErrorIs(t, err, context.Canceled)

	err = s.Save(ctx, "orders", 0, 10)
	require.ErrorIs(t, err, context.Canceled)
}
