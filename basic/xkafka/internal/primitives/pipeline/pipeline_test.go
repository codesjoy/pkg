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

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposeError(t *testing.T) {
	t.Parallel()

	order := make([]string, 0, 3)
	chain := ComposeError(
		[]func(context.Context, *string, func(context.Context, *string) error) error{
			func(ctx context.Context, s *string, next func(context.Context, *string) error) error {
				order = append(order, "h1")
				return next(ctx, s)
			},
			func(ctx context.Context, s *string, next func(context.Context, *string) error) error {
				order = append(order, "h2")
				return next(ctx, s)
			},
		},
		func(context.Context, *string) error {
			order = append(order, "final")
			return nil
		},
	)

	require.NoError(t, chain(context.Background(), nil))
	require.Equal(t, []string{"h1", "h2", "final"}, order)
}

func TestComposeResult(t *testing.T) {
	t.Parallel()

	chain := ComposeResult(
		[]func(context.Context, *string, func(context.Context, *string) (*int, error)) (*int, error){
			func(ctx context.Context, s *string, next func(context.Context, *string) (*int, error)) (*int, error) {
				return next(ctx, s)
			},
		},
		func(context.Context, *string) (*int, error) {
			v := 1
			return &v, nil
		},
	)

	result, err := chain(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, *result)
}
