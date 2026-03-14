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
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"
)

func encodeMessage(stream string, cfg PublisherConfig, msg *Message) *redis.XAddArgs {
	values := make([]interface{}, 0, 2+len(msg.Headers)*2)
	values = append(values, cfg.PayloadField, string(msg.Payload))

	headerKeys := make([]string, 0, len(msg.Headers))
	for key := range msg.Headers {
		headerKeys = append(headerKeys, key)
	}
	sort.Strings(headerKeys)
	for _, key := range headerKeys {
		values = append(values, cfg.HeaderPrefix+key, msg.Headers[key])
	}

	return &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}
}

func decodeMessage(stream string, headerPrefix string, payloadField string, raw redis.XMessage) *Message {
	msg := &Message{
		Stream:  stream,
		Payload: nil,
	}

	for key, value := range raw.Values {
		switch {
		case key == payloadField:
			msg.Payload = asBytes(value)
		case strings.HasPrefix(key, headerPrefix):
			if msg.Headers == nil {
				msg.Headers = make(map[string]string)
			}
			msg.Headers[strings.TrimPrefix(key, headerPrefix)] = asString(value)
		}
	}

	return msg
}
