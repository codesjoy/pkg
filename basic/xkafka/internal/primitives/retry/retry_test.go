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

package retry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAndBackoff(t *testing.T) {
	t.Parallel()

	defaults := Config{
		MaxRetries:     InfiniteRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     4 * time.Millisecond,
		Multiplier:     2,
	}
	cfg := NormalizeConfig(Config{MaxRetries: InfiniteRetries}, defaults)
	require.Equal(t, defaults, cfg)
	require.Equal(t, time.Millisecond, Backoff(cfg, 1))
	require.Equal(t, 2*time.Millisecond, Backoff(cfg, 2))
	require.Equal(t, 4*time.Millisecond, Backoff(cfg, 3))
	require.Equal(t, 4*time.Millisecond, Backoff(cfg, 4))
}

func TestIsExhausted(t *testing.T) {
	t.Parallel()

	cfg := Config{
		MaxRetries:     2,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Second,
		Multiplier:     2,
	}
	require.False(t, IsExhausted(cfg, 1))
	require.False(t, IsExhausted(cfg, 2))
	require.True(t, IsExhausted(cfg, 3))
}
