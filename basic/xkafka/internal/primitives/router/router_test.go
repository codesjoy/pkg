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

package router

import (
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

func TestDefaultConsumeKeyExtractor(t *testing.T) {
	t.Parallel()

	_, err := DefaultConsumeKeyExtractor(nil)
	require.Error(t, err)

	key, err := DefaultConsumeKeyExtractor(&sarama.ConsumerMessage{
		Topic:     "orders",
		Partition: 2,
		Key:       []byte("order-1"),
	})
	require.NoError(t, err)
	require.Equal(t, "order-1", key)

	key, err = DefaultConsumeKeyExtractor(&sarama.ConsumerMessage{
		Topic:     "orders",
		Partition: 2,
	})
	require.NoError(t, err)
	require.Equal(t, "orders:2", key)
}

func TestShardForKeyStable(t *testing.T) {
	t.Parallel()

	first := ShardForKey("same-key", 32)
	for i := 0; i < 100; i++ {
		require.Equal(t, first, ShardForKey("same-key", 32))
	}
}

func TestProduceDispatchKey(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", ProduceDispatchKey(nil))
	require.Equal(t, "k", ProduceDispatchKey(&produce.Message{Topic: "orders", Key: []byte("k")}))
	require.Equal(t, "orders", ProduceDispatchKey(&produce.Message{Topic: "orders"}))
}
