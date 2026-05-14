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

package sarama

import (
	"context"
	"errors"
	"testing"
	"time"

	ibmsarama "github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

func TestSyncProducerSenderSendBatchReport(t *testing.T) {
	now := time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC)
	sender := &SyncProducerSender{
		producer: &fakeSyncProducer{
			sendMessagesFn: func(msgs []*ibmsarama.ProducerMessage) error {
				var producerErrors ibmsarama.ProducerErrors
				for i, msg := range msgs {
					key, _ := msg.Key.Encode()
					if string(key) == "2" {
						producerErrors = append(producerErrors, &ibmsarama.ProducerError{
							Msg: msg,
							Err: errors.New("send failed"),
						})
						continue
					}
					msg.Partition = int32(i)
					msg.Offset = int64(i + 1)
					msg.Timestamp = now
				}
				if len(producerErrors) > 0 {
					return producerErrors
				}
				return nil
			},
		},
	}

	results, err := sender.SendBatchReport(context.Background(), []*produce.Message{
		{Topic: "orders", Key: []byte("1"), Value: []byte("a")},
		{Topic: "orders", Key: []byte("2"), Value: []byte("b")},
		{Topic: "orders", Key: []byte("3"), Value: []byte("c")},
	})
	require.NoError(t, err)
	require.Len(t, results, 3)
	require.NotNil(t, results[0].Result)
	require.NoError(t, results[0].Err)
	require.Equal(t, int32(0), results[0].Result.Partition)
	require.Equal(t, int64(1), results[0].Result.Offset)
	require.Equal(t, now, results[0].Result.Timestamp)
	require.Nil(t, results[1].Result)
	require.EqualError(t, results[1].Err, "send message: send failed")
	require.NotNil(t, results[2].Result)
	require.NoError(t, results[2].Err)
	require.Equal(t, int32(2), results[2].Result.Partition)
	require.Equal(t, int64(3), results[2].Result.Offset)
}

func TestSyncProducerSenderSendBatchReportCanceledBeforeStart(t *testing.T) {
	sender := &SyncProducerSender{producer: &fakeSyncProducer{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := sender.SendBatchReport(context.Background(), nil)
	require.Nil(t, results)
	require.NoError(t, err)

	results, err = sender.SendBatchReport(ctx, []*produce.Message{
		{Topic: "orders", Value: []byte("a")},
	})
	require.Nil(t, results)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNewSyncProducerSenderUsesInjectedProducer(t *testing.T) {
	producer := &fakeSyncProducer{}

	sender, err := NewSyncProducerSender(SyncProducerConfig{Producer: producer})
	require.NoError(t, err)
	require.Same(t, producer, sender.producer)
	require.False(t, sender.owned)
}

func TestNewSyncProducerSenderRejectsMissingBrokers(t *testing.T) {
	sender, err := NewSyncProducerSender(SyncProducerConfig{})
	require.Nil(t, sender)
	require.EqualError(t, err, "brokers are required when sync producer is nil")
}

func TestSyncProducerSenderSendValidatesInputs(t *testing.T) {
	result, err := (*SyncProducerSender)(nil).Send(context.Background(), &produce.Message{})
	require.Nil(t, result)
	require.EqualError(t, err, "sync producer is not configured")

	sender := &SyncProducerSender{producer: &fakeSyncProducer{}}
	result, err = sender.Send(context.Background(), nil)
	require.Nil(t, result)
	require.EqualError(t, err, "producer message is nil")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = sender.Send(ctx, &produce.Message{Topic: "orders"})
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSyncProducerSenderSend(t *testing.T) {
	now := time.Date(2026, 4, 11, 11, 0, 0, 0, time.UTC)
	producer := &fakeSyncProducer{
		sendMessageFn: func(msg *ibmsarama.ProducerMessage) (int32, int64, error) {
			require.Equal(t, "orders", msg.Topic)
			key, err := msg.Key.Encode()
			require.NoError(t, err)
			require.Equal(t, []byte("order-1"), key)
			value, err := msg.Value.Encode()
			require.NoError(t, err)
			require.Equal(t, []byte("payload"), value)
			require.Equal(t, now, msg.Timestamp)
			require.Len(t, msg.Headers, 1)
			return 3, 42, nil
		},
	}
	sender := &SyncProducerSender{producer: producer}

	result, err := sender.Send(context.Background(), &produce.Message{
		Topic:     "orders",
		Key:       []byte("order-1"),
		Value:     []byte("payload"),
		Timestamp: now,
		Headers: []ibmsarama.RecordHeader{{
			Key:   []byte("trace"),
			Value: []byte("abc"),
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "orders", result.Topic)
	require.Equal(t, int32(3), result.Partition)
	require.Equal(t, int64(42), result.Offset)
	require.Equal(t, now, result.Timestamp)
	require.NotNil(t, producer.lastMessage)
}

func TestSyncProducerSenderSendWrapsProducerError(t *testing.T) {
	sender := &SyncProducerSender{
		producer: &fakeSyncProducer{
			sendMessageFn: func(*ibmsarama.ProducerMessage) (int32, int64, error) {
				return 0, 0, errors.New("broker down")
			},
		},
	}

	result, err := sender.Send(context.Background(), &produce.Message{Topic: "orders"})
	require.Nil(t, result)
	require.EqualError(t, err, "send message: broker down")
}

func TestSyncProducerSenderCloseOnlyOwnedProducer(t *testing.T) {
	producer := &fakeSyncProducer{}
	require.NoError(t, (&SyncProducerSender{producer: producer}).Close())
	require.Equal(t, 0, producer.closeCalls)

	sender := &SyncProducerSender{producer: producer, owned: true}
	require.NoError(t, sender.Close())
	require.Equal(t, 1, producer.closeCalls)
}

type fakeSyncProducer struct {
	sendMessageFn  func(*ibmsarama.ProducerMessage) (int32, int64, error)
	sendMessagesFn func([]*ibmsarama.ProducerMessage) error
	closeFn        func() error
	lastMessage    *ibmsarama.ProducerMessage
	closeCalls     int
}

func (f *fakeSyncProducer) SendMessage(msg *ibmsarama.ProducerMessage) (int32, int64, error) {
	f.lastMessage = msg
	if f.sendMessageFn != nil {
		return f.sendMessageFn(msg)
	}
	return 0, 0, nil
}

func (f *fakeSyncProducer) SendMessages(msgs []*ibmsarama.ProducerMessage) error {
	if f.sendMessagesFn != nil {
		return f.sendMessagesFn(msgs)
	}
	return nil
}

func (f *fakeSyncProducer) Close() error {
	f.closeCalls++
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func (*fakeSyncProducer) TxnStatus() ibmsarama.ProducerTxnStatusFlag { return 0 }

func (*fakeSyncProducer) IsTransactional() bool { return false }

func (*fakeSyncProducer) BeginTxn() error { return nil }

func (*fakeSyncProducer) CommitTxn() error { return nil }

func (*fakeSyncProducer) AbortTxn() error { return nil }

func (*fakeSyncProducer) AddOffsetsToTxn(
	_ map[string][]*ibmsarama.PartitionOffsetMetadata,
	_ string,
) error {
	return nil
}

func (*fakeSyncProducer) AddMessageToTxn(
	_ *ibmsarama.ConsumerMessage,
	_ string,
	_ *string,
) error {
	return nil
}
