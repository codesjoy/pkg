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

package xnats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

func TestPublisherBuildPublishChainModes(t *testing.T) {
	cfg := defaultPublisherConfig()
	publisherInstance := &Publisher{cfg: cfg}

	var appendOrder []string
	publisherInstance.cfg.GlobalHandlers = []publish.Handler{
		newPublishRecorder("global", &appendOrder),
	}
	publisherInstance.cfg.SubjectHandlers["append"] = PublishSubjectHandlers{
		Mode:     ChainModeAppend,
		Handlers: []publish.Handler{newPublishRecorder("subject", &appendOrder)},
	}

	_, err := publisherInstance.buildPublishChain("append", func(
		_ context.Context,
		_ *publish.MessageContext,
	) (*publish.Result, error) {
		appendOrder = append(appendOrder, "business")
		return &publish.Result{}, nil
	})(context.Background(), &publish.MessageContext{})
	require.NoError(t, err)
	require.Equal(t, []string{"global", "subject", "business"}, appendOrder)

	var replaceOrder []string
	publisherInstance.cfg.GlobalHandlers = []publish.Handler{
		newPublishRecorder("global", &replaceOrder),
	}
	publisherInstance.cfg.SubjectHandlers["replace"] = PublishSubjectHandlers{
		Mode:     ChainModeReplace,
		Handlers: []publish.Handler{newPublishRecorder("subject", &replaceOrder)},
	}

	_, err = publisherInstance.buildPublishChain("replace", func(
		_ context.Context,
		_ *publish.MessageContext,
	) (*publish.Result, error) {
		replaceOrder = append(replaceOrder, "business")
		return &publish.Result{}, nil
	})(context.Background(), &publish.MessageContext{})
	require.NoError(t, err)
	require.Equal(t, []string{"subject", "business"}, replaceOrder)
}

func TestSubscriberBuildConsumeChainModes(t *testing.T) {
	cfg := defaultSubscriberConfig()
	subscriberInstance := &Subscriber{cfg: cfg}

	var appendOrder []string
	subscriberInstance.cfg.GlobalHandlers = []consume.Handler{
		newConsumeRecorder("global", &appendOrder),
	}
	subscriberInstance.cfg.SubjectHandlers["append"] = ConsumeSubjectHandlers{
		Mode:     ChainModeAppend,
		Handlers: []consume.Handler{newConsumeRecorder("subject", &appendOrder)},
	}

	err := subscriberInstance.buildConsumeChain("append", func(
		_ context.Context,
		_ *consume.MessageContext,
	) error {
		appendOrder = append(appendOrder, "business")
		return nil
	})(context.Background(), &consume.MessageContext{})
	require.NoError(t, err)
	require.Equal(t, []string{"global", "subject", "business"}, appendOrder)

	var replaceOrder []string
	subscriberInstance.cfg.GlobalHandlers = []consume.Handler{
		newConsumeRecorder("global", &replaceOrder),
	}
	subscriberInstance.cfg.SubjectHandlers["replace"] = ConsumeSubjectHandlers{
		Mode:     ChainModeReplace,
		Handlers: []consume.Handler{newConsumeRecorder("subject", &replaceOrder)},
	}

	err = subscriberInstance.buildConsumeChain("replace", func(
		_ context.Context,
		_ *consume.MessageContext,
	) error {
		replaceOrder = append(replaceOrder, "business")
		return nil
	})(context.Background(), &consume.MessageContext{})
	require.NoError(t, err)
	require.Equal(t, []string{"subject", "business"}, replaceOrder)
}

func newPublishRecorder(name string, order *[]string) publish.Handler {
	return publish.Func(func(
		ctx context.Context,
		msg *publish.MessageContext,
		next publish.Next,
	) (*publish.Result, error) {
		*order = append(*order, name)
		return next(ctx, msg)
	})
}

func newConsumeRecorder(name string, order *[]string) consume.Handler {
	return consume.Func(
		func(ctx context.Context, msg *consume.MessageContext, next consume.Next) error {
			*order = append(*order, name)
			return next(ctx, msg)
		},
	)
}
