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

package pipeline

import "context"

// ComposeError composes a no-result middleware chain.
func ComposeError[T any](
	handlers []func(context.Context, *T, func(context.Context, *T) error) error,
	final func(context.Context, *T) error,
) func(context.Context, *T) error {
	chained := final
	for i := len(handlers) - 1; i >= 0; i-- {
		h := handlers[i]
		if h == nil {
			continue
		}
		next := chained
		chained = func(ctx context.Context, item *T) error {
			return h(ctx, item, next)
		}
	}
	return chained
}

// ComposeResult composes a middleware chain with one typed result.
func ComposeResult[T any, R any](
	handlers []func(context.Context, *T, func(context.Context, *T) (*R, error)) (*R, error),
	final func(context.Context, *T) (*R, error),
) func(context.Context, *T) (*R, error) {
	chained := final
	for i := len(handlers) - 1; i >= 0; i-- {
		h := handlers[i]
		if h == nil {
			continue
		}
		next := chained
		chained = func(ctx context.Context, item *T) (*R, error) {
			return h(ctx, item, next)
		}
	}
	return chained
}
