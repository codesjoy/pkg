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
	"fmt"
	"time"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/backoff"
)

const (
	// InfiniteRetries means retry forever.
	InfiniteRetries = -1
)

// Config controls retry behavior.
// 重试行为的通用配置。
type Config struct {
	// MaxRetries 是最大重试次数，-1 表示无限重试。
	MaxRetries int
	// InitialBackoff 是首次重试的等待时长。
	InitialBackoff time.Duration
	// MaxBackoff 是重试等待的最大时长上限。
	MaxBackoff time.Duration
	// Multiplier 是指数退避的乘数因子。
	Multiplier float64
}

// NormalizeConfig fills zero-values with defaults.
// 将配置的零值字段替换为默认值，并校正不合理参数。
func NormalizeConfig(cfg Config, defaults Config) Config {
	n := cfg
	// 补填 InitialBackoff 默认值
	if n.InitialBackoff <= 0 {
		n.InitialBackoff = defaults.InitialBackoff
	}
	// 补填 MaxBackoff 默认值
	if n.MaxBackoff <= 0 {
		n.MaxBackoff = defaults.MaxBackoff
	}
	// 确保 MaxBackoff >= InitialBackoff
	if n.MaxBackoff < n.InitialBackoff {
		n.MaxBackoff = n.InitialBackoff
	}
	// 补填 Multiplier 默认值
	if n.Multiplier <= 0 {
		n.Multiplier = defaults.Multiplier
	}
	// 约束 Multiplier >= 1
	if n.Multiplier < 1 {
		n.Multiplier = 1
	}
	// 约束 MaxRetries >= InfiniteRetries (-1)
	if n.MaxRetries < InfiniteRetries {
		n.MaxRetries = InfiniteRetries
	}
	return n
}

// ValidateConfig validates retry settings.
func ValidateConfig(cfg Config) error {
	if cfg.MaxRetries < InfiniteRetries {
		return fmt.Errorf("max retries must be >= %d, got %d", InfiniteRetries, cfg.MaxRetries)
	}
	if cfg.InitialBackoff <= 0 {
		return fmt.Errorf("initial backoff must be > 0, got %s", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff <= 0 {
		return fmt.Errorf("max backoff must be > 0, got %s", cfg.MaxBackoff)
	}
	if cfg.MaxBackoff < cfg.InitialBackoff {
		return fmt.Errorf(
			"max backoff (%s) must be >= initial backoff (%s)",
			cfg.MaxBackoff,
			cfg.InitialBackoff,
		)
	}
	if cfg.Multiplier < 1 {
		return fmt.Errorf("multiplier must be >= 1, got %f", cfg.Multiplier)
	}
	return nil
}

// IsExhausted reports whether current attempt reaches finite retry limits.
// 判断当前尝试次数是否已达到有限重试上限。无限重试模式永远返回 false。
func IsExhausted(cfg Config, attempt int) bool {
	// 无限重试模式永远不会耗尽
	if cfg.MaxRetries == InfiniteRetries {
		return false
	}
	return attempt >= cfg.MaxRetries+1
}

// Backoff returns retry backoff duration for the attempt.
func Backoff(cfg Config, attempt int) time.Duration {
	return backoff.Exponential(cfg.InitialBackoff, cfg.MaxBackoff, cfg.Multiplier, attempt)
}
