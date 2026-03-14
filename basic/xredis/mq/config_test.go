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

package mq

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestPublisherConfigValidateDefaults(t *testing.T) {
	t.Parallel()

	var cfg PublisherConfig
	require.NoError(t, cfg.Validate())
	require.Equal(t, defaultPayloadField, cfg.PayloadField)
	require.Equal(t, defaultHeaderPrefix, cfg.HeaderPrefix)
	require.Equal(t, defaultOrderKeyHeader, cfg.OrderKeyHeader)
	require.Equal(t, 0, cfg.OrderedShardCount)
}

func TestConsumerConfigValidateDefaults(t *testing.T) {
	t.Parallel()

	cfg := ConsumerConfig{
		Stream:   " jobs ",
		Group:    " workers ",
		Consumer: " c1 ",
	}
	require.NoError(t, cfg.Validate())
	require.Equal(t, "jobs", cfg.Stream)
	require.Equal(t, "workers", cfg.Group)
	require.Equal(t, "c1", cfg.Consumer)
	require.Equal(t, defaultBlock, cfg.Block)
	require.Equal(t, defaultCount, cfg.Count)
	require.True(t, cfg.AutoCreateGroup)
	require.Equal(t, defaultGroupStartID, cfg.GroupStartID)
	require.Equal(t, defaultPayloadField, cfg.PayloadField)
	require.Equal(t, defaultHeaderPrefix, cfg.HeaderPrefix)
	require.Equal(t, defaultOrderKeyHeader, cfg.OrderKeyHeader)
	require.Equal(t, defaultShardCount, cfg.ShardCount)
	require.Equal(t, defaultShardQueueSize, cfg.ShardQueueSize)
	require.Equal(t, 0, cfg.OrderedShardCount)
	require.Equal(t, defaultAutoClaimMinIdle, cfg.AutoClaimMinIdle)
	require.Equal(t, defaultAutoClaimCount, cfg.AutoClaimCount)
	require.Equal(t, defaultIdleBackoff, cfg.IdleBackoff)
}

func TestConsumerConfigValidateErrors(t *testing.T) {
	t.Parallel()

	cfg := ConsumerConfig{}
	require.ErrorIs(t, cfg.Validate(), ErrConsumerStreamRequired)

	cfg = ConsumerConfig{Stream: "jobs"}
	require.ErrorIs(t, cfg.Validate(), ErrConsumerGroupRequired)

	cfg = ConsumerConfig{Stream: "jobs", Group: "workers"}
	require.ErrorIs(t, cfg.Validate(), ErrConsumerNameRequired)

	cfg = ConsumerConfig{Stream: "jobs", Group: "workers", Consumer: "c1", Count: -1}
	require.Error(t, cfg.Validate())

	cfg = ConsumerConfig{Stream: "jobs", Group: "workers", Consumer: "c1", ShardCount: -1}
	require.Error(t, cfg.Validate())

	cfg = ConsumerConfig{Stream: "jobs", Group: "workers", Consumer: "c1", ShardQueueSize: -1}
	require.Error(t, cfg.Validate())

	cfg = ConsumerConfig{Stream: "jobs", Group: "workers", Consumer: "c1", OrderedShardCount: -1}
	require.Error(t, cfg.Validate())

	cfg = ConsumerConfig{Stream: "jobs", Group: "workers", Consumer: "c1", OwnedShards: []int{0}}
	require.Error(t, cfg.Validate())

	cfg = ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c1",
		OrderedShardCount: 2,
		OwnedShards:       []int{1, 0, 1},
	}
	require.NoError(t, cfg.Validate())
	require.Equal(t, []int{0, 1}, cfg.OwnedShards)

	cfg = ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c1",
		OrderedShardCount: 2,
		OwnedShards:       []int{2},
	}
	require.Error(t, cfg.Validate())

	cfg = ConsumerConfig{Stream: "jobs", Group: "workers", Consumer: "c1", Block: -time.Second}
	require.Error(t, cfg.Validate())
}

func TestDecodeMessageConvertsBasicTypes(t *testing.T) {
	t.Parallel()

	raw := map[string]interface{}{
		defaultPayloadField:       42,
		defaultHeaderPrefix + "k": []byte("v"),
	}

	msg := decodeMessage(
		"jobs",
		defaultHeaderPrefix,
		defaultPayloadField,
		redis.XMessage{Values: raw},
	)

	require.Equal(t, "jobs", msg.Stream)
	require.Equal(t, []byte("42"), msg.Payload)
	require.Equal(t, map[string]string{"k": "v"}, msg.Headers)
}
