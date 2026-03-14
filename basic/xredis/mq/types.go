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

package mq

import (
	"context"
	"time"
)

// Message is one logical Redis Streams message.
type Message struct {
	Stream  string
	Payload []byte
	Headers map[string]string
}

// PublishResult is one successful publish result.
type PublishResult struct {
	BaseStream string
	Stream     string
	Shard      int
	ID         string
	Published  time.Time
}

// MessageContext contains per-message metadata passed to the business handler.
type MessageContext struct {
	Message       *Message
	BaseStream    string
	ShardStream   string
	Stream        string
	ID            string
	Group         string
	Consumer      string
	LogicalKey    string
	Shard         int
	DeliveryCount int64
	Claimed       bool
	ReceivedAt    time.Time
}

// HandlerFunc handles one consumed message.
type HandlerFunc func(context.Context, *MessageContext) error
