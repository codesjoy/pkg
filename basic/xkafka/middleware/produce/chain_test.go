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

package produce

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposeOrder(t *testing.T) {
	t.Parallel()

	order := make([]string, 0, 3)
	handlers := []Handler{
		Func(func(ctx context.Context, msg *MessageContext, next Next) (*Result, error) {
			order = append(order, "h1")
			return next(ctx, msg)
		}),
		Func(func(ctx context.Context, msg *MessageContext, next Next) (*Result, error) {
			order = append(order, "h2")
			return next(ctx, msg)
		}),
	}

	chain := Compose(handlers, func(context.Context, *MessageContext) (*Result, error) {
		order = append(order, "business")
		return &Result{Topic: "orders", Partition: 0, Offset: 1}, nil
	})

	result, err := chain(context.Background(), &MessageContext{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, []string{"h1", "h2", "business"}, order)
}

func TestComposeShortCircuit(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("stop")
	chain := Compose([]Handler{
		Func(func(context.Context, *MessageContext, Next) (*Result, error) {
			return nil, wantErr
		}),
		Func(func(ctx context.Context, msg *MessageContext, next Next) (*Result, error) {
			return next(ctx, msg)
		}),
	}, func(context.Context, *MessageContext) (*Result, error) {
		return &Result{}, nil
	})

	result, err := chain(context.Background(), &MessageContext{})
	require.Nil(t, result)
	require.ErrorIs(t, err, wantErr)
}

func TestComposeNilFinalHandler(t *testing.T) {
	t.Parallel()

	chain := Compose(nil, nil)
	result, err := chain(context.Background(), &MessageContext{})
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrNilHandlerFunc)
}
