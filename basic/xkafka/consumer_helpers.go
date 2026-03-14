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

func prepareConsumeCall(
	isNil bool,
	nilReceiverErr string,
	ctx context.Context,
	business consume.HandlerFunc,
) (context.Context, error) {
	if isNil {
		return nil, errors.New(nilReceiverErr)
	}
	if business == nil {
		return nil, consume.ErrNilHandlerFunc
	}
	return normalizeContext(ctx), nil
}

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
	if boolValue(loggerHandlerEnabled, true) {
		handlers = append(handlers, clogger.New(logger))
	}
	return append(handlers, retryHandler)
}

func extractConsumeLogicalKey(
	extractor KeyExtractor,
	msg *sarama.ConsumerMessage,
) (string, error) {
	key, err := extractor(msg)
	if err != nil {
		return "", err
	}
	if key == "" {
		return router.ConsumeFallbackKey(msg), nil
	}
	return key, nil
}

func selectTopicHandlers[T any](
	global []T,
	mode ChainMode,
	topicHandlers []T,
) []T {
	if mode == ChainModeReplace {
		return topicHandlers
	}
	return append(append([]T(nil), global...), topicHandlers...)
}
