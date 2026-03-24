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

package logger

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
)

const defaultSlowThreshold = 200 * time.Millisecond

// Config controls logger monitor behavior.
type Config struct {
	Logger          *slog.Logger
	SlowThreshold   time.Duration
	LogCommandBody  bool
	CommandFilter   func(commandName string) bool
	PoolEventFilter func(eventType string) bool
}

// Monitors groups logger monitors for xmongo.
type Monitors struct {
	Command *event.CommandMonitor
	Pool    *event.PoolMonitor
	Server  *event.ServerMonitor
}

// DefaultConfig returns the default logger config.
func DefaultConfig() Config {
	return Config{
		Logger:        slog.Default(),
		SlowThreshold: defaultSlowThreshold,
	}
}

type requestKey struct {
	ConnectionID string
	RequestID    int64
}

type startedCommand struct {
	CommandName  string
	DatabaseName string
	CommandBody  string
}

type monitorLogger struct {
	logger          *slog.Logger
	slowThreshold   time.Duration
	logCommandBody  bool
	commandFilter   func(string) bool
	poolEventFilter func(string) bool
	started         sync.Map
}

// New creates slog-backed MongoDB monitors.
func New(cfg Config) Monitors {
	normalized := normalizeConfig(cfg)
	m := &monitorLogger{
		logger:          normalized.Logger,
		slowThreshold:   normalized.SlowThreshold,
		logCommandBody:  normalized.LogCommandBody,
		commandFilter:   normalized.CommandFilter,
		poolEventFilter: normalized.PoolEventFilter,
	}
	return Monitors{
		Command: m.commandMonitor(),
		Pool:    m.poolMonitor(),
		Server:  m.serverMonitor(),
	}
}

func (m *monitorLogger) commandMonitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started: func(_ context.Context, evt *event.CommandStartedEvent) { m.recordStartedCommand(evt) },
		Succeeded: func(ctx context.Context, evt *event.CommandSucceededEvent) {
			m.logCommandSucceeded(ctx, evt)
		},
		Failed: func(ctx context.Context, evt *event.CommandFailedEvent) {
			m.logCommandFailed(ctx, evt)
		},
	}
}

func (m *monitorLogger) poolMonitor() *event.PoolMonitor {
	return &event.PoolMonitor{
		Event: m.logPoolEvent,
	}
}

func (m *monitorLogger) serverMonitor() *event.ServerMonitor {
	return &event.ServerMonitor{
		ServerHeartbeatFailed: m.logServerHeartbeatFailed,
	}
}

func (m *monitorLogger) recordStartedCommand(evt *event.CommandStartedEvent) {
	if evt == nil {
		return
	}
	m.started.Store(
		requestKey{ConnectionID: evt.ConnectionID, RequestID: evt.RequestID},
		startedCommand{
			CommandName:  evt.CommandName,
			DatabaseName: evt.DatabaseName,
			CommandBody:  commandBody(evt.Command),
		},
	)
}

func (m *monitorLogger) logCommandSucceeded(
	ctx context.Context,
	evt *event.CommandSucceededEvent,
) {
	if evt == nil {
		return
	}
	if m.shouldSkipCommand(evt.CommandName) || evt.Duration < m.slowThreshold {
		m.deleteStarted(evt.ConnectionID, evt.RequestID)
		return
	}
	m.logger.WarnContext(
		ctx,
		"xmongo command slow",
		m.completeCommandAttrs(
			evt.CommandName,
			evt.Duration,
			evt.ConnectionID,
			evt.RequestID,
		)...,
	)
}

func (m *monitorLogger) logCommandFailed(
	ctx context.Context,
	evt *event.CommandFailedEvent,
) {
	if evt == nil {
		return
	}
	if m.shouldSkipCommand(evt.CommandName) {
		m.deleteStarted(evt.ConnectionID, evt.RequestID)
		return
	}

	attrs := m.completeCommandAttrs(
		evt.CommandName,
		evt.Duration,
		evt.ConnectionID,
		evt.RequestID,
	)
	if evt.Failure != nil {
		attrs = append(attrs, slog.String("error", evt.Failure.Error()))
	}
	m.logger.ErrorContext(ctx, "xmongo command failed", attrs...)
}

func (m *monitorLogger) logPoolEvent(evt *event.PoolEvent) {
	if evt == nil || !m.shouldLogPoolEvent(evt) {
		return
	}

	attrs := m.poolEventAttrs(evt)
	if evt.Error != nil {
		m.logger.Error("xmongo pool event", attrs...)
		return
	}
	m.logger.Warn("xmongo pool event", attrs...)
}

func (m *monitorLogger) logServerHeartbeatFailed(evt *event.ServerHeartbeatFailedEvent) {
	if evt == nil {
		return
	}
	m.logger.Error("xmongo server heartbeat failed", m.serverHeartbeatFailedAttrs(evt)...)
}

func (m *monitorLogger) shouldSkipCommand(commandName string) bool {
	return m.commandFilter != nil && m.commandFilter(commandName)
}

func (m *monitorLogger) shouldLogPoolEvent(evt *event.PoolEvent) bool {
	if evt == nil {
		return false
	}
	if m.poolEventFilter != nil && m.poolEventFilter(evt.Type) {
		return false
	}
	switch evt.Type {
	case event.ConnectionPoolCleared, event.ConnectionPoolClosed, event.ConnectionCheckOutFailed:
		return true
	default:
		return false
	}
}

func (m *monitorLogger) commandAttrs(
	commandName string,
	duration time.Duration,
	connectionID string,
	requestID int64,
	start startedCommand,
) []any {
	attrs := []any{
		slog.String("command", commandName),
		slog.Duration("duration", duration),
		slog.String("connection_id", connectionID),
		slog.Int64("request_id", requestID),
	}
	if start.DatabaseName != "" {
		attrs = append(attrs, slog.String("database", start.DatabaseName))
	}
	if m.logCommandBody && start.CommandBody != "" {
		attrs = append(attrs, slog.String("command_body", start.CommandBody))
	}
	return attrs
}

func (m *monitorLogger) completeCommandAttrs(
	commandName string,
	duration time.Duration,
	connectionID string,
	requestID int64,
) []any {
	return m.commandAttrs(
		commandName,
		duration,
		connectionID,
		requestID,
		m.deleteStarted(connectionID, requestID),
	)
}

func (m *monitorLogger) poolEventAttrs(evt *event.PoolEvent) []any {
	attrs := []any{
		slog.String("event_type", evt.Type),
		slog.String("address", evt.Address),
		slog.Int64("connection_id", evt.ConnectionID),
	}
	if evt.Reason != "" {
		attrs = append(attrs, slog.String("reason", evt.Reason))
	}
	if evt.Duration > 0 {
		attrs = append(attrs, slog.Duration("duration", evt.Duration))
	}
	if evt.Error != nil {
		attrs = append(attrs, slog.String("error", evt.Error.Error()))
	}
	return attrs
}

func (m *monitorLogger) serverHeartbeatFailedAttrs(
	evt *event.ServerHeartbeatFailedEvent,
) []any {
	attrs := []any{
		slog.String("connection_id", evt.ConnectionID),
		slog.Duration("duration", evt.Duration),
		slog.Bool("awaited", evt.Awaited),
	}
	if evt.Failure != nil {
		attrs = append(attrs, slog.String("error", evt.Failure.Error()))
	}
	return attrs
}

func (m *monitorLogger) deleteStarted(connectionID string, requestID int64) startedCommand {
	key := requestKey{ConnectionID: connectionID, RequestID: requestID}
	started, ok := m.started.LoadAndDelete(key)
	if !ok {
		return startedCommand{}
	}
	cached, _ := started.(startedCommand)
	return cached
}

func normalizeConfig(cfg Config) Config {
	normalized := cfg
	if normalized.Logger == nil {
		normalized.Logger = slog.Default()
	}
	if normalized.SlowThreshold <= 0 {
		normalized.SlowThreshold = defaultSlowThreshold
	}
	return normalized
}

func commandBody(raw bson.Raw) string {
	if len(raw) == 0 {
		return ""
	}
	return fmt.Sprint(raw)
}
