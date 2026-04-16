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

package debeziumgorm_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	debeziumgorm "github.com/codesjoy/pkg/basic/xevent/outbox/debezium/gorm"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type exampleEvent struct {
	ID      string `json:"id"`
	OrderID string `json:"order_id"`
}

func (*exampleEvent) EventType() string {
	return "order.created"
}

func (e *exampleEvent) EventID() string {
	return e.ID
}

func (e *exampleEvent) PartitionKey() string {
	return e.OrderID
}

func (*exampleEvent) Topic() string {
	return ""
}

func (e *exampleEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *exampleEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

func ExampleAppendEvent() {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	if err := createExampleSchema(db); err != nil {
		panic(err)
	}

	store, err := debeziumgorm.NewGORMStore(debeziumgorm.GORMStoreConfig{DB: db})
	if err != nil {
		panic(err)
	}

	record, err := debezium.AppendEvent(context.Background(), store, &exampleEvent{
		ID:      "evt_1",
		OrderID: "order-1",
	}, debezium.AppendOptions{Topic: "orders"})
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s %s\n", record.Topic, record.EventType)

	// Output:
	// orders order.created
}

func createExampleSchema(db *gorm.DB) error {
	return db.Exec(`
CREATE TABLE xevent_outbox_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL,
  handoff_from_id INTEGER NULL UNIQUE,
  topic TEXT NOT NULL,
  partition_key TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL,
  event_id TEXT NOT NULL DEFAULT '',
  payload BLOB NOT NULL,
  available_at DATETIME NOT NULL,
  status TEXT NOT NULL DEFAULT '',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  claim_owner TEXT NOT NULL DEFAULT '',
  claim_until DATETIME NULL,
  sent_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX idx_xevent_outbox_mode_status_partition_available_id
  ON xevent_outbox_records (mode, status, partition_key, available_at, id);
CREATE INDEX idx_xevent_outbox_mode_created_at
  ON xevent_outbox_records (mode, created_at);
`).Error
}
