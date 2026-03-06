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
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
)

func TestPartitionConsumerConfigValidateRequired(t *testing.T) {
	t.Parallel()

	cfg := PartitionConsumerConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "brokers are required")

	cfg.Brokers = []string{"127.0.0.1:9092"}
	err = cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "topic is required")

	cfg.Topic = "orders"
	cfg.Partition = -1
	err = cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "partition must be >= 0")
}

func TestPartitionConsumerConfigDefaultOffsetStore(t *testing.T) {
	t.Parallel()

	cfg := PartitionConsumerConfig{
		Brokers:   []string{"127.0.0.1:9092"},
		Topic:     "orders",
		Partition: 0,
	}

	require.NoError(t, cfg.Validate())
	require.NotNil(t, cfg.OffsetStore)
}

func TestPartitionConsumerConfigInitialOffsetValidate(t *testing.T) {
	t.Parallel()

	cfg := PartitionConsumerConfig{
		Brokers:       []string{"127.0.0.1:9092"},
		Topic:         "orders",
		Partition:     0,
		InitialOffset: sarama.OffsetOldest - 1,
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "initial offset")
}

func TestPartitionConsumerConfigReconnectNormalize(t *testing.T) {
	t.Parallel()

	cfg := PartitionConsumerConfig{
		Brokers:   []string{"127.0.0.1:9092"},
		Topic:     "orders",
		Partition: 0,
		Reconnect: BackoffConfig{
			InitialBackoff: 0,
			MaxBackoff:     time.Millisecond,
			Multiplier:     0,
		},
	}

	require.NoError(t, cfg.Validate())
	require.Greater(t, cfg.Reconnect.InitialBackoff, time.Duration(0))
	require.GreaterOrEqual(t, cfg.Reconnect.MaxBackoff, cfg.Reconnect.InitialBackoff)
	require.GreaterOrEqual(t, cfg.Reconnect.Multiplier, 1.0)
}

func TestPartitionConsumerConfigValidateDLQAndPolicy(t *testing.T) {
	t.Parallel()

	t.Run("rejects unsupported exhausted policy", func(t *testing.T) {
		cfg := PartitionConsumerConfig{
			Brokers:         []string{"127.0.0.1:9092"},
			Topic:           "orders",
			Partition:       0,
			ExhaustedPolicy: "invalid",
		}

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported exhausted policy")
	})

	t.Run("requires dlq config when dlq commit is enabled", func(t *testing.T) {
		cfg := PartitionConsumerConfig{
			Brokers:         []string{"127.0.0.1:9092"},
			Topic:           "orders",
			Partition:       0,
			ExhaustedPolicy: ExhaustedPolicyDLQCommit,
		}

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "DLQ config is required")
	})

	t.Run("requires dlq topic", func(t *testing.T) {
		cfg := PartitionConsumerConfig{
			Brokers:         []string{"127.0.0.1:9092"},
			Topic:           "orders",
			Partition:       0,
			ExhaustedPolicy: ExhaustedPolicyDLQCommit,
			DLQ:             &DLQConfig{Topic: "   "},
		}

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "DLQ topic is required")
	})
}

func TestPartitionConsumerConfigValidateDefaultsDependencies(t *testing.T) {
	t.Parallel()

	cfg := PartitionConsumerConfig{
		Brokers:   []string{"127.0.0.1:9092"},
		Topic:     "orders",
		Partition: 0,
	}

	require.NoError(t, cfg.Validate())
	require.NotNil(t, cfg.KeyExtractor)
	require.NotNil(t, cfg.Logger)
	require.NotNil(t, cfg.SaramaConfig)
	require.NotNil(t, cfg.OffsetStore)
	require.NotNil(t, cfg.LoggerHandlerEnabled)
	require.Equal(t, ExhaustedPolicyBlock, cfg.ExhaustedPolicy)
}
