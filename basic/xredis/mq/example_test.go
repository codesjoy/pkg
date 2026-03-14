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

package mq_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/codesjoy/pkg/basic/xredis/mq"
)

func Example_publishAndConsume() {
	mr, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	publisher, err := mq.NewPublisher(client, mq.PublisherConfig{DefaultStream: "jobs"})
	if err != nil {
		panic(err)
	}
	consumer, err := mq.NewConsumer(client, mq.ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "worker-1",
		AutoCreateGroup:   true,
		OrderedShardCount: 4,
		OwnedShards:       []int{mqTestShard("user-42", 4)},
		Block:             10 * time.Millisecond,
		IdleBackoff:       5 * time.Millisecond,
		AutoClaimMinIdle:  time.Second,
	})
	if err != nil {
		panic(err)
	}

	publisher, err = mq.NewPublisher(client, mq.PublisherConfig{
		DefaultStream:     "jobs",
		OrderedShardCount: 4,
	})
	if err != nil {
		panic(err)
	}

	if _, err := publisher.Publish(context.Background(), &mq.Message{
		Payload: []byte("send-email"),
		Headers: map[string]string{
			"kind":        "welcome",
			"x-order-key": "user-42",
		},
	}); err != nil {
		panic(err)
	}

	_ = consumer.Consume(
		context.Background(),
		func(_ context.Context, msg *mq.MessageContext) error {
			fmt.Printf("%s %s\n", msg.Message.Payload, msg.Message.Headers["kind"])
			return consumer.Close()
		},
	)

	// Output:
	// send-email welcome
}

func mqTestShard(key string, shardCount int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(shardCount))
}
