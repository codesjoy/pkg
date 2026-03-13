package xnats

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubscriberConfigValidate(t *testing.T) {
	cfg := SubscriberConfig{
		URLs:       []string{" nats://127.0.0.1:4222 "},
		Subjects:   []string{" orders.created ", ""},
		QueueGroup: " workers ",
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, []string{"nats://127.0.0.1:4222"}, cfg.URLs)
	require.Equal(t, []string{"orders.created"}, cfg.Subjects)
	require.Equal(t, "workers", cfg.QueueGroup)
	require.Equal(t, ConsumeExhaustedPolicyBlock, cfg.ExhaustedPolicy)
}

func TestJetStreamConsumerConfigDefaults(t *testing.T) {
	cfg := JetStreamConsumerConfig{
		URLs:     []string{"nats://127.0.0.1:4222"},
		Stream:   "ORDERS",
		Consumer: "worker",
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, JetStreamConsumerModePull, cfg.Mode)
	require.Equal(t, DefaultPullBatchSize, cfg.PullBatchSize)
	require.Equal(t, DefaultPullMaxWait, cfg.PullMaxWait)
	require.Equal(t, DefaultPullIdleBackoff, cfg.IdleBackoff)
}
