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
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Publisher wraps Redis Streams publish helpers.
type Publisher struct {
	client redis.UniversalClient
	cfg    PublisherConfig
}

// NewPublisher creates a configured publisher wrapper.
func NewPublisher(client redis.UniversalClient, cfg PublisherConfig) (*Publisher, error) {
	if isNilClient(client) {
		return nil, ErrNilClient
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Publisher{client: client, cfg: cfg}, nil
}

// Publish sends one message synchronously.
func (p *Publisher) Publish(ctx context.Context, msg *Message) (*PublishResult, error) {
	if p == nil {
		return nil, ErrNilPublisher
	}
	if msg == nil {
		return nil, ErrNilMessage
	}
	ctx = normalizeContext(ctx)

	// Resolve the target shard stream for this message.
	binding, _, err := orderedPublishBinding(p.cfg, msg)
	if err != nil {
		return nil, err
	}

	// Encode and send via XADD.
	id, err := p.client.XAdd(ctx, encodeMessage(binding.ShardStream, p.cfg, msg)).Result()
	if err != nil {
		return nil, err
	}

	return &PublishResult{
		BaseStream: binding.BaseStream,
		Stream:     binding.ShardStream,
		Shard:      binding.Shard,
		ID:         id,
		Published:  time.Now(),
	}, nil
}

// PublishBatch sends messages sequentially and fails fast on first error.
func (p *Publisher) PublishBatch(
	ctx context.Context,
	msgs ...*Message,
) ([]*PublishResult, error) {
	if p == nil {
		return nil, ErrNilPublisher
	}
	ctx = normalizeContext(ctx)
	// Return early for empty input.
	if len(msgs) == 0 {
		return nil, nil
	}

	// Publish each message; stop on the first error.
	results := make([]*PublishResult, len(msgs))
	for i, msg := range msgs {
		result, err := p.Publish(ctx, msg)
		if err != nil {
			return results, fmt.Errorf("publish batch index %d: %w", i, err)
		}
		results[i] = result
	}
	return results, nil
}
