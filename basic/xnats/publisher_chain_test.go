package xnats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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
