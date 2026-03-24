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

package xmongo

import (
	"context"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

var pingClient = func(
	client *mongo.Client,
	ctx context.Context,
	rp *readpref.ReadPref,
) error {
	if client == nil {
		return mongo.ErrClientDisconnected
	}
	return client.Ping(ctx, rp)
}

// HealthSnapshot is a lightweight snapshot of recent client health signals.
type HealthSnapshot struct {
	LastPingAt        time.Time
	LastPingErr       error
	LastPoolEventType string
	LastPoolEventAt   time.Time
	LastHeartbeatErr  string
	LastHeartbeatAt   time.Time
	UpdatedAt         time.Time
}

type healthTracker struct {
	mu        sync.RWMutex
	snapshotv HealthSnapshot
}

func newHealthTracker() *healthTracker {
	return &healthTracker{}
}

// PingPrimary performs an explicit readiness check against the primary.
func (c *Client) PingPrimary(ctx context.Context) error {
	err := pingClient(c.Raw(), ctx, readpref.Primary())
	if c != nil && c.healthTracker != nil {
		c.healthTracker.recordPing(time.Now(), err)
	}
	return err
}

// HealthSnapshot returns the latest lightweight health snapshot.
func (c *Client) HealthSnapshot() HealthSnapshot {
	if c == nil || c.healthTracker == nil {
		return HealthSnapshot{}
	}
	return c.healthTracker.snapshot()
}

func (t *healthTracker) recordPing(at time.Time, err error) {
	t.update(at, func(snapshot *HealthSnapshot) {
		snapshot.LastPingAt = at
		snapshot.LastPingErr = err
	})
}

func (t *healthTracker) poolMonitor() *event.PoolMonitor {
	if t == nil {
		return nil
	}
	return &event.PoolMonitor{
		Event: func(evt *event.PoolEvent) {
			if evt == nil {
				return
			}
			at := time.Now()
			t.update(at, func(snapshot *HealthSnapshot) {
				snapshot.LastPoolEventType = evt.Type
				snapshot.LastPoolEventAt = at
			})
		},
	}
}

func (t *healthTracker) serverMonitor() *event.ServerMonitor {
	if t == nil {
		return nil
	}
	return &event.ServerMonitor{
		ServerHeartbeatSucceeded: func(evt *event.ServerHeartbeatSucceededEvent) {
			if evt == nil {
				return
			}
			at := time.Now()
			t.update(at, func(snapshot *HealthSnapshot) {
				snapshot.LastHeartbeatAt = at
				snapshot.LastHeartbeatErr = ""
			})
		},
		ServerHeartbeatFailed: func(evt *event.ServerHeartbeatFailedEvent) {
			if evt == nil {
				return
			}
			at := time.Now()
			t.update(at, func(snapshot *HealthSnapshot) {
				snapshot.LastHeartbeatAt = at
				if evt.Failure != nil {
					snapshot.LastHeartbeatErr = evt.Failure.Error()
					return
				}
				snapshot.LastHeartbeatErr = ""
			})
		},
	}
}

func (t *healthTracker) update(at time.Time, fn func(*HealthSnapshot)) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	fn(&t.snapshotv)
	t.snapshotv.UpdatedAt = at
}

func (t *healthTracker) snapshot() HealthSnapshot {
	if t == nil {
		return HealthSnapshot{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.snapshotv
}
