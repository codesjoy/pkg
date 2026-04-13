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
	"log/slog"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/router"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	clogger "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/logger"
)

// prepareConsumeCall 统一的消费调用前置检查：
// nil receiver 检查、nil handler 检查、context 规范化。
func prepareConsumeCall(
	isNil bool,
	nilReceiverErr string,
	ctx context.Context,
	business consume.HandlerFunc,
) (context.Context, error) {
	// nil receiver 检查
	if isNil {
		return nil, errors.New(nilReceiverErr)
	}
	// nil handler 检查
	if business == nil {
		return nil, consume.ErrNilHandlerFunc
	}
	return normalizeContext(ctx), nil
}

// baseConsumeHandlers 构建基础的消费者中间件链：
// 可选的日志中间件 + 必选的重试中间件。
func baseConsumeHandlers(
	logger *slog.Logger,
	loggerHandlerEnabled *bool,
	retryHandler consume.Handler,
	additionalCapacity int,
) []consume.Handler {
	capacity := additionalCapacity + 1
	if boolValue(loggerHandlerEnabled, true) {
		capacity++
	}

	handlers := make([]consume.Handler, 0, capacity)
	// 按需添加日志中间件
	if boolValue(loggerHandlerEnabled, true) {
		handlers = append(handlers, clogger.New(logger))
	}
	// 添加重试中间件
	return append(handlers, retryHandler)
}

// extractConsumeLogicalKey 调用提取器获取逻辑键，空值时回退到 topic:partition。
func extractConsumeLogicalKey(
	extractor KeyExtractor,
	msg *sarama.ConsumerMessage,
) (string, error) {
	// 调用用户自定义或默认的键提取器
	key, err := extractor(msg)
	if err != nil {
		return "", err
	}
	// 空值回退到 topic:partition 格式
	if key == "" {
		return router.ConsumeFallbackKey(msg), nil
	}
	return key, nil
}

// selectTopicHandlers 根据 chain mode 决定如何组合全局处理器和 topic 处理器：
// Replace 模式只使用 topic 处理器，Append 模式将 topic 处理器追加到全局处理器之后。
func selectTopicHandlers[T any](
	global []T,
	mode ChainMode,
	topicHandlers []T,
) []T {
	// Replace 模式：完全替换全局处理器
	if mode == ChainModeReplace {
		return topicHandlers
	}
	// Append 模式：追加到全局处理器之后
	return append(append([]T(nil), global...), topicHandlers...)
}
