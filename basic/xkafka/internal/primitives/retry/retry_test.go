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

func TestNormalizeConfig(t *testing.T) {
	t.Parallel()

	defaults := Config{
		MaxRetries:     3,
		InitialBackoff: 2 * time.Millisecond,
		MaxBackoff:     8 * time.Millisecond,
		Multiplier:     2,
	}

	tests := []struct {
		name string
		in   Config
		want Config
	}{
		{
			name: "fills zero values with defaults",
			in:   Config{},
			want: Config{
				MaxRetries:     0,
				InitialBackoff: defaults.InitialBackoff,
				MaxBackoff:     defaults.MaxBackoff,
				Multiplier:     defaults.Multiplier,
			},
		},
		{
			name: "normalizes invalid ranges",
			in: Config{
				MaxRetries:     -3,
				InitialBackoff: -1,
				MaxBackoff:     time.Millisecond,
				Multiplier:     0.5,
			},
			want: Config{
				MaxRetries:     InfiniteRetries,
				InitialBackoff: defaults.InitialBackoff,
				MaxBackoff:     defaults.InitialBackoff,
				Multiplier:     1,
			},
		},
		{
			name: "raises max backoff to initial backoff",
			in: Config{
				MaxRetries:     1,
				InitialBackoff: 5 * time.Millisecond,
				MaxBackoff:     time.Millisecond,
				Multiplier:     3,
			},
			want: Config{
				MaxRetries:     1,
				InitialBackoff: 5 * time.Millisecond,
				MaxBackoff:     5 * time.Millisecond,
				Multiplier:     3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeConfig(tt.in, defaults))
		})
	}
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid",
			cfg: Config{
				MaxRetries:     1,
				InitialBackoff: time.Millisecond,
				MaxBackoff:     2 * time.Millisecond,
				Multiplier:     2,
			},
		},
		{
			name: "invalid retries",
			cfg: Config{
				MaxRetries:     -2,
				InitialBackoff: time.Millisecond,
				MaxBackoff:     2 * time.Millisecond,
				Multiplier:     2,
			},
			wantErr: "max retries must be >=",
		},
		{
			name: "invalid initial backoff",
			cfg: Config{
				MaxRetries:     1,
				InitialBackoff: 0,
				MaxBackoff:     2 * time.Millisecond,
				Multiplier:     2,
			},
			wantErr: "initial backoff must be > 0",
		},
		{
			name: "invalid max backoff",
			cfg: Config{
				MaxRetries:     1,
				InitialBackoff: time.Millisecond,
				MaxBackoff:     0,
				Multiplier:     2,
			},
			wantErr: "max backoff must be > 0",
		},
		{
			name: "max backoff lower than initial",
			cfg: Config{
				MaxRetries:     1,
				InitialBackoff: 2 * time.Millisecond,
				MaxBackoff:     time.Millisecond,
				Multiplier:     2,
			},
			wantErr: "must be >=",
		},
		{
			name: "invalid multiplier",
			cfg: Config{
				MaxRetries:     1,
				InitialBackoff: time.Millisecond,
				MaxBackoff:     2 * time.Millisecond,
				Multiplier:     0.5,
			},
			wantErr: "multiplier must be >=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
