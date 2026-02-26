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

package consume

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposeOrder(t *testing.T) {
	t.Parallel()

	var order []string
	handlers := []Handler{
		Func(func(ctx context.Context, msg *MessageContext, next Next) error {
			order = append(order, "h1-before")
			err := next(ctx, msg)
			order = append(order, "h1-after")
			return err
		}),
		Func(func(ctx context.Context, msg *MessageContext, next Next) error {
			order = append(order, "h2-before")
			err := next(ctx, msg)
			order = append(order, "h2-after")
			return err
		}),
	}

	chain := Compose(handlers, func(context.Context, *MessageContext) error {
		order = append(order, "business")
		return nil
	})

	require.NoError(t, chain(context.Background(), &MessageContext{}))
	require.Equal(t, []string{"h1-before", "h2-before", "business", "h2-after", "h1-after"}, order)
}

func TestComposeShortCircuit(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("stop")
	called := false

	chain := Compose([]Handler{
		Func(func(context.Context, *MessageContext, Next) error {
			return wantErr
		}),
	}, func(context.Context, *MessageContext) error {
		called = true
		return nil
	})

	err := chain(context.Background(), &MessageContext{})
	require.ErrorIs(t, err, wantErr)
	require.False(t, called)
}

func TestComposeNilFinalHandler(t *testing.T) {
	t.Parallel()

	chain := Compose(nil, nil)
	err := chain(context.Background(), &MessageContext{})
	require.ErrorIs(t, err, ErrNilHandlerFunc)
}
