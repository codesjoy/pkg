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

// Package produce provides producer-side middleware types and composition helpers.
package produce

import (
	"context"
	"errors"
	"time"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/pipeline"
)

// ErrNilHandlerFunc indicates no final message producer handler is provided.
var ErrNilHandlerFunc = errors.New("produce handler is nil")

// Message is one logical message to produce.
// 待发送的一条 Kafka 消息。
type Message struct {
	// Topic 是目标 topic 名称。
	Topic string
	// Key 是消息键，用于分区路由。
	Key []byte
	// Value 是消息体。
	Value []byte
	// Headers 是消息头列表。
	Headers []sarama.RecordHeader
	// Timestamp 是消息时间戳，零值由 broker 分配。
	Timestamp time.Time
}

// Result is one successful produce result.
// 成功发送后 broker 返回的结果。
type Result struct {
	// Topic 是消息被发送到的 topic。
	Topic string
	// Partition 是消息被分配到的分区。
	Partition int32
	// Offset 是消息在分区中的偏移量。
	Offset int64
	// Timestamp 是 broker 分配的时间戳。
	Timestamp time.Time
	// Attempt 是实际发送尝试次数。
	Attempt int
}

// BatchItemResult reports the per-item outcome of one batch produce call.
// 批量发送中每条消息的独立结果。
type BatchItemResult struct {
	// Result 是发送成功的元数据，失败时为 nil。
	Result *Result
	// Err 是发送失败的错误，成功时为 nil。
	Err error
}

// Future is an async result handle for one produce call.
type Future interface {
	Await(context.Context) (*Result, error)
	Done() <-chan struct{}
}

// MessageContext contains per-message metadata passed through handlers.
// 生产者中间件链中传递的每条消息上下文。
type MessageContext struct {
	// Message 是待发送的消息。
	Message *Message
	// Attempt 是当前发送尝试次数（含首次）。
	Attempt int
	// ReceivedAt 是消息进入中间件链的时间。
	ReceivedAt time.Time
	// DispatchKey 是用于异步分发的路由键。
	DispatchKey string
	// Worker 是实际执行发送的工作协程索引。
	Worker int
}

// HandlerFunc handles one producer message and returns a broker result.
type HandlerFunc func(context.Context, *MessageContext) (*Result, error)

// Next is the next handler in a chain.
type Next func(context.Context, *MessageContext) (*Result, error)

// Handler is a chain middleware for produce handling.
type Handler interface {
	Handle(context.Context, *MessageContext, Next) (*Result, error)
}

// Func adapts a function into a Handler.
type Func func(context.Context, *MessageContext, Next) (*Result, error)

// Handle executes the adapted handler function.
func (f Func) Handle(
	ctx context.Context,
	msg *MessageContext,
	next Next,
) (*Result, error) {
	if f == nil {
		return next(ctx, msg)
	}
	return f(ctx, msg, next)
}

// Compose builds a middleware chain around a final business handler.
// 从处理器列表构建中间件链：过滤 nil handler，反向嵌套，最终调用 business handler。
func Compose(handlers []Handler, final HandlerFunc) HandlerFunc {
	if final == nil {
		return func(context.Context, *MessageContext) (*Result, error) {
			return nil, ErrNilHandlerFunc
		}
	}

	// 将 Handler 接口适配为统一的函数签名
	adapted := make(
		[]func(context.Context, *MessageContext, func(context.Context, *MessageContext) (*Result, error)) (*Result, error),
		0,
		len(handlers),
	)
	for _, handler := range handlers {
		// 过滤 nil handler
		if handler == nil {
			continue
		}
		current := handler
		adapted = append(adapted, func(
			ctx context.Context,
			msg *MessageContext,
			next func(context.Context, *MessageContext) (*Result, error),
		) (*Result, error) {
			return current.Handle(ctx, msg, Next(next))
		})
	}

	// 反向构建嵌套链
	return pipeline.ComposeResult(adapted, final)
}
