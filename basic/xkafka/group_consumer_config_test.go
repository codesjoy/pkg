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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupConsumerConfigValidateRequired(t *testing.T) {
	t.Parallel()

	cfg := GroupConsumerConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "brokers are required")

	cfg.Brokers = []string{"127.0.0.1:9092"}
	err = cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "group ID is required")

	cfg.GroupID = "group"
	err = cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "topics are required")
}

func TestGroupConsumerConfigTopicHandlerModeDefault(t *testing.T) {
	t.Parallel()

	cfg := GroupConsumerConfig{
		Brokers: []string{"127.0.0.1:9092"},
		GroupID: "group",
		Topics:  []string{"orders"},
		TopicHandlers: map[string]ConsumeTopicHandlers{
			"orders": {},
		},
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, ChainModeAppend, cfg.TopicHandlers["orders"].Mode)
}
