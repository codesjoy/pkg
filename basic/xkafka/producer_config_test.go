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

package xkafka

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProducerConfigValidateRequired(t *testing.T) {
	t.Parallel()

	cfg := ProducerConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "producer brokers are required")
}

func TestProducerConfigDispatchNormalize(t *testing.T) {
	t.Parallel()

	cfg := ProducerConfig{Brokers: []string{"127.0.0.1:9092"}}
	require.NoError(t, cfg.Validate())
	require.Equal(t, ProducerDispatchModeKeySharded, cfg.Dispatch.Mode)
	require.Equal(t, DefaultShardCount, cfg.Dispatch.ShardCount)
	require.Equal(t, DefaultShardQueueSize, cfg.Dispatch.QueueSize)
}

func TestProducerConfigDispatchModeValidate(t *testing.T) {
	t.Parallel()

	cfg := ProducerConfig{
		Brokers: []string{"127.0.0.1:9092"},
		Dispatch: ProducerDispatchConfig{
			Mode:        "invalid",
			QueueSize:   1,
			ShardCount:  1,
			WorkerCount: 1,
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported producer dispatch mode")
}

func TestProducerConfigValidatePoliciesAndTopicHandlers(t *testing.T) {
	t.Parallel()

	t.Run("rejects unsupported exhausted policy", func(t *testing.T) {
		cfg := ProducerConfig{
			Brokers:         []string{"127.0.0.1:9092"},
			ExhaustedPolicy: "invalid",
		}

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported producer exhausted policy")
	})

	t.Run("defaults topic handler mode to append", func(t *testing.T) {
		cfg := ProducerConfig{
			Brokers: []string{"127.0.0.1:9092"},
			TopicHandlers: map[string]ProduceTopicHandlers{
				"orders": {},
			},
		}

		require.NoError(t, cfg.Validate())
		require.Equal(t, ChainModeAppend, cfg.TopicHandlers["orders"].Mode)
	})

	t.Run("rejects empty topic handler key", func(t *testing.T) {
		cfg := ProducerConfig{
			Brokers: []string{"127.0.0.1:9092"},
			TopicHandlers: map[string]ProduceTopicHandlers{
				"   ": {},
			},
		}

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty topic")
	})

	t.Run("rejects unsupported topic handler mode", func(t *testing.T) {
		cfg := ProducerConfig{
			Brokers: []string{"127.0.0.1:9092"},
			TopicHandlers: map[string]ProduceTopicHandlers{
				"orders": {Mode: "invalid"},
			},
		}

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported chain mode")
	})
}

func TestProducerConfigValidateDispatchModes(t *testing.T) {
	t.Parallel()

	t.Run("serial dispatch", func(t *testing.T) {
		cfg := ProducerConfig{
			Brokers: []string{"127.0.0.1:9092"},
			Dispatch: ProducerDispatchConfig{
				Mode: ProducerDispatchModeSerial,
			},
		}

		require.NoError(t, cfg.Validate())
		require.Equal(t, ProducerDispatchModeSerial, cfg.Dispatch.Mode)
		require.Equal(t, DefaultShardQueueSize, cfg.Dispatch.QueueSize)
	})

	t.Run("parallel dispatch normalizes worker count", func(t *testing.T) {
		cfg := ProducerConfig{
			Brokers: []string{"127.0.0.1:9092"},
			Dispatch: ProducerDispatchConfig{
				Mode: ProducerDispatchModeParallel,
			},
		}

		require.NoError(t, cfg.Validate())
		require.Equal(t, DefaultProducerWorkerCount, cfg.Dispatch.WorkerCount)
	})
}
