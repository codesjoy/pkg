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

type fakeSyncProducer struct {
	sendMessagesFn func([]*ibmsarama.ProducerMessage) error
}

func (f *fakeSyncProducer) SendMessage(_ *ibmsarama.ProducerMessage) (int32, int64, error) {
	return 0, 0, nil
}

func (f *fakeSyncProducer) SendMessages(msgs []*ibmsarama.ProducerMessage) error {
	if f.sendMessagesFn != nil {
		return f.sendMessagesFn(msgs)
	}
	return nil
}

func (*fakeSyncProducer) Close() error { return nil }

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
