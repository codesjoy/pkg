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

package sarama

import (
	"fmt"

	ibmsarama "github.com/IBM/sarama"
)

// NewConsumerGroup creates a Sarama consumer group client.
func NewConsumerGroup(
	brokers []string,
	groupID string,
	cfg *ibmsarama.Config,
) (ibmsarama.ConsumerGroup, error) {
	group, err := ibmsarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, fmt.Errorf("create consumer group: %w", err)
	}
	return group, nil
}
