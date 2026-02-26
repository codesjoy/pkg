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
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

func TestNewProducerValidate(t *testing.T) {
	t.Parallel()

	_, err := NewProducer(ProducerConfig{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "producer brokers are required")
}

func TestProducerProduceDefaultTopic(t *testing.T) {
	t.Parallel()

	enabled := false
	mock := &fakeProducerSyncProducer{}
	producerInstance, err := NewProducer(ProducerConfig{
		Brokers:              []string{"127.0.0.1:9092"},
		SyncProducer:         mock,
		DefaultTopic:         "orders",
		LoggerHandlerEnabled: &enabled,
		RetryConfig: RetryConfig{
			MaxRetries:     0,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
			Multiplier:     1,
		},
		ExhaustedPolicy: ProducerExhaustedPolicyStop,
	})
	require.NoError(t, err)
	defer producerInstance.Close()

	result, err := producerInstance.Produce(
		context.Background(),
		&produce.Message{Value: []byte("v")},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "orders", result.Topic)
	require.NotNil(t, mock.lastMsg)
	require.Equal(t, "orders", mock.lastMsg.Topic)
}

func TestProducerProduceNilMessage(t *testing.T) {
	t.Parallel()

	enabled := false
	mock := &fakeProducerSyncProducer{}
	producerInstance, err := NewProducer(ProducerConfig{
		Brokers:              []string{"127.0.0.1:9092"},
		SyncProducer:         mock,
		LoggerHandlerEnabled: &enabled,
	})
	require.NoError(t, err)
	defer producerInstance.Close()

	result, err := producerInstance.Produce(context.Background(), nil)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrNilProducerMessage)
}

func TestProducerProduceBatchFailFast(t *testing.T) {
	t.Parallel()

	enabled := false
	mock := &fakeProducerSyncProducer{
		sendFn: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			if msg.Key != nil {
				encoded, _ := msg.Key.Encode()
				if string(encoded) == "2" {
					return 0, 0, errors.New("send failed")
				}
			}
			return 0, 1, nil
		},
	}
	producerInstance, err := NewProducer(ProducerConfig{
		Brokers:              []string{"127.0.0.1:9092"},
		SyncProducer:         mock,
		DefaultTopic:         "orders",
		LoggerHandlerEnabled: &enabled,
		RetryConfig: RetryConfig{
			MaxRetries:     0,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
			Multiplier:     1,
		},
		ExhaustedPolicy: ProducerExhaustedPolicyStop,
	})
	require.NoError(t, err)
	defer producerInstance.Close()

	results, err := producerInstance.ProduceBatch(
		context.Background(),
		&produce.Message{Key: []byte("1"), Value: []byte("a")},
		&produce.Message{Key: []byte("2"), Value: []byte("b")},
		&produce.Message{Key: []byte("3"), Value: []byte("c")},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "produce batch index 1")
	require.Len(t, results, 3)
	require.NotNil(t, results[0])
	require.Nil(t, results[1])
	require.Nil(t, results[2])
}

func TestProducerProduceAsyncAndClose(t *testing.T) {
	t.Parallel()

	enabled := false
	mock := &fakeProducerSyncProducer{}
	producerInstance, err := NewProducer(ProducerConfig{
		Brokers:              []string{"127.0.0.1:9092"},
		SyncProducer:         mock,
		DefaultTopic:         "orders",
		LoggerHandlerEnabled: &enabled,
		Dispatch: ProducerDispatchConfig{
			Mode:      ProducerDispatchModeSerial,
			QueueSize: 8,
		},
	})
	require.NoError(t, err)

	future, err := producerInstance.ProduceAsync(
		context.Background(),
		&produce.Message{Value: []byte("v")},
	)
	require.NoError(t, err)
	result, err := future.Await(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NoError(t, producerInstance.Close())
	require.NoError(t, producerInstance.Close())

	_, err = producerInstance.ProduceAsync(
		context.Background(),
		&produce.Message{Value: []byte("v")},
	)
	require.ErrorIs(t, err, ErrProducerClosed)
}

func TestProducerProduceAsyncQueueBackpressure(t *testing.T) {
	t.Parallel()

	enabled := false
	block := make(chan struct{})
	mock := &fakeProducerSyncProducer{
		sendFn: func(*sarama.ProducerMessage) (int32, int64, error) {
			<-block
			return 0, 0, nil
		},
	}
	producerInstance, err := NewProducer(ProducerConfig{
		Brokers:              []string{"127.0.0.1:9092"},
		SyncProducer:         mock,
		DefaultTopic:         "orders",
		LoggerHandlerEnabled: &enabled,
		Dispatch: ProducerDispatchConfig{
			Mode:      ProducerDispatchModeSerial,
			QueueSize: 1,
		},
	})
	require.NoError(t, err)
	defer producerInstance.Close()

	_, err = producerInstance.ProduceAsync(
		context.Background(),
		&produce.Message{Value: []byte("1")},
	)
	require.NoError(t, err)
	_, err = producerInstance.ProduceAsync(
		context.Background(),
		&produce.Message{Value: []byte("2")},
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = producerInstance.ProduceAsync(ctx, &produce.Message{Value: []byte("3")})
	require.ErrorIs(t, err, context.DeadlineExceeded)

	close(block)
}

type fakeProducerSyncProducer struct {
	mu sync.Mutex

	lastMsg    *sarama.ProducerMessage
	sendErr    error
	sendFn     func(*sarama.ProducerMessage) (int32, int64, error)
	closeCalls int
}

func (m *fakeProducerSyncProducer) SendMessage(msg *sarama.ProducerMessage) (int32, int64, error) {
	m.mu.Lock()
	m.lastMsg = msg
	sendFn := m.sendFn
	sendErr := m.sendErr
	m.mu.Unlock()

	if sendFn != nil {
		return sendFn(msg)
	}
	if sendErr != nil {
		return 0, 0, sendErr
	}
	return 1, 2, nil
}

func (m *fakeProducerSyncProducer) SendMessages(_ []*sarama.ProducerMessage) error {
	return nil
}

func (m *fakeProducerSyncProducer) Close() error {
	m.mu.Lock()
	m.closeCalls++
	m.mu.Unlock()
	return nil
}

func (m *fakeProducerSyncProducer) TxnStatus() sarama.ProducerTxnStatusFlag {
	return 0
}

func (m *fakeProducerSyncProducer) IsTransactional() bool {
	return false
}

func (m *fakeProducerSyncProducer) BeginTxn() error {
	return nil
}

func (m *fakeProducerSyncProducer) CommitTxn() error {
	return nil
}

func (m *fakeProducerSyncProducer) AbortTxn() error {
	return nil
}

func (m *fakeProducerSyncProducer) AddOffsetsToTxn(
	_ map[string][]*sarama.PartitionOffsetMetadata,
	_ string,
) error {
	return nil
}

func (m *fakeProducerSyncProducer) AddMessageToTxn(
	_ *sarama.ConsumerMessage,
	_ string,
	_ *string,
) error {
	return nil
}
