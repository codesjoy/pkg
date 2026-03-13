package xnats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

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

func newConsumeRecorder(name string, order *[]string) consume.Handler {
	return consume.Func(
		func(ctx context.Context, msg *consume.MessageContext, next consume.Next) error {
			*order = append(*order, name)
			return next(ctx, msg)
		},
	)
}
