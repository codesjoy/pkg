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

//go:build integration

package integration

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xredis"
)

func TestStandaloneSetGet(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client, err := xredis.New(
		xredis.Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mustAddr(t)}}},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	require.NoError(t, client.Set(ctx, "it:standalone:key", "value", 0).Err())
	value, err := client.Get(ctx, "it:standalone:key").Result()
	require.NoError(t, err)
	require.Equal(t, "value", value)
}
