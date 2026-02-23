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

// Package xgo provides panic-safe goroutine helpers.
package xgo

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// PanicInfo contains panic metadata captured from a goroutine.
type PanicInfo struct {
	Recovered any
	Stack     []byte
	Ctx       context.Context
}

// PanicHandler is invoked when a goroutine panics.
type PanicHandler func(PanicInfo)

// Option customizes a Runner.
type Option func(*Runner)

// Runner runs functions in goroutines and handles panic recovery.
type Runner struct {
	logger       *slog.Logger
	panicHandler PanicHandler
}

var defaultRunner = New()

// New creates a Runner with optional custom logger and panic hook.
func New(opts ...Option) *Runner {
	r := &Runner{
		logger: slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

// Default returns the process-wide default runner.
func Default() *Runner {
	return defaultRunner
}

// WithLogger sets the panic logger. Pass nil to disable panic logging.
func WithLogger(logger *slog.Logger) Option {
	return func(r *Runner) {
		r.logger = logger
	}
}

// WithPanicHandler sets a callback invoked after panic recovery.
func WithPanicHandler(handler PanicHandler) Option {
	return func(r *Runner) {
		r.panicHandler = handler
	}
}

// Go runs f in a new goroutine and recovers panic.
func Go(f func()) {
	Default().Go(f)
}

// GoWithCtx runs f in a new goroutine with context and recovers panic.
func GoWithCtx(ctx context.Context, f func(context.Context)) {
	Default().GoWithCtx(ctx, f)
}

// Go runs f in a new goroutine and recovers panic.
func (r *Runner) Go(f func()) {
	if f == nil {
		return
	}

	go r.run(context.Background(), func(context.Context) {
		f()
	})
}

// GoWithCtx runs f in a new goroutine with ctx and recovers panic.
func (r *Runner) GoWithCtx(ctx context.Context, f func(context.Context)) {
	if f == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	go r.run(ctx, f)
}

func (r *Runner) run(ctx context.Context, f func(context.Context)) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		stack := debug.Stack()
		info := PanicInfo{
			Recovered: recovered,
			Stack:     stack,
			Ctx:       ctx,
		}

		if r.logger != nil {
			r.logger.Error(
				"goroutine panic",
				slog.Any("panic", recovered),
				slog.String("stack", string(stack)),
			)
		}
		if r.panicHandler != nil {
			r.panicHandler(info)
		}
	}()

	f(ctx)
}
