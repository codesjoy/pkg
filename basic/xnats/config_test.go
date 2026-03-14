package xnats

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

type testContextKey struct{}

func TestConfigHelpers(t *testing.T) {
	t.Run("normalizeStrings", func(t *testing.T) {
		require.Nil(t, normalizeStrings(nil))
		require.Equal(t, []string{"a", "b"}, normalizeStrings([]string{" a ", "", "b"}))
	})

	t.Run("normalizeContext", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), testContextKey{}, "v")
		require.Same(t, ctx, normalizeContext(ctx))
		require.NotNil(t, normalizeContext(context.TODO()))
	})

	t.Run("boolValue", func(t *testing.T) {
		require.True(t, boolValue(nil, true))
		v := false
		require.False(t, boolValue(&v, true))
	})
}

func TestConnectionHelpers(t *testing.T) {
	t.Run("connect requires urls", func(t *testing.T) {
		conn, err := connect(nil, nil)
		require.Nil(t, conn)
		require.EqualError(t, err, "nats URLs are required")
	})

	t.Run("newJetStream nil conn", func(t *testing.T) {
		js, err := newJetStream(nil)
		require.Nil(t, js)
		require.ErrorIs(t, err, ErrJetStreamRequired)
	})

	t.Run("drain helpers tolerate nil", func(t *testing.T) {
		require.NoError(t, drainConnection(nil))
		require.NoError(t, drainSubscriptions(nil))
	})
}

func TestPublisherConfigValidate(t *testing.T) {
	cfg := PublisherConfig{
		URLs:           []string{" nats://127.0.0.1:4222 ", ""},
		DefaultSubject: " orders ",
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, []string{"nats://127.0.0.1:4222"}, cfg.URLs)
	require.Equal(t, "orders", cfg.DefaultSubject)
	require.Equal(t, PublishExhaustedPolicyBlock, cfg.ExhaustedPolicy)
	require.NotNil(t, cfg.Logger)
	require.NotNil(t, cfg.LoggerHandlerEnabled)
}

func TestPublisherConfigValidateSubjectHandlers(t *testing.T) {
	cfg := PublisherConfig{
		URLs: []string{"nats://127.0.0.1:4222"},
		SubjectHandlers: map[string]PublishSubjectHandlers{
			"": {},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty subject")
}

func TestPublisherConfigValidateAdditionalBranches(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var cfg *PublisherConfig
		err := cfg.Validate()
		require.EqualError(t, err, "publisher config is nil")
	})

	t.Run("invalid exhausted policy", func(t *testing.T) {
		cfg := PublisherConfig{
			URLs:            []string{"nats://127.0.0.1:4222"},
			ExhaustedPolicy: PublishExhaustedPolicy("bad"),
		}
		err := cfg.Validate()
		require.EqualError(t, err, `unsupported publish exhausted policy "bad"`)
	})

	t.Run("invalid chain mode", func(t *testing.T) {
		cfg := PublisherConfig{
			URLs: []string{"nats://127.0.0.1:4222"},
			SubjectHandlers: map[string]PublishSubjectHandlers{
				"orders": {Mode: ChainMode("bad")},
			},
		}
		err := cfg.Validate()
		require.EqualError(t, err, `publish subject "orders" uses unsupported chain mode "bad"`)
	})

	t.Run("default append mode", func(t *testing.T) {
		cfg := PublisherConfig{
			URLs: []string{"nats://127.0.0.1:4222"},
			SubjectHandlers: map[string]PublishSubjectHandlers{
				"orders": {},
			},
		}
		require.NoError(t, cfg.Validate())
		require.Equal(t, ChainModeAppend, cfg.SubjectHandlers["orders"].Mode)
	})

	t.Run("conn satisfies dependency", func(t *testing.T) {
		cfg := PublisherConfig{Conn: &nats.Conn{}}
		require.NoError(t, cfg.Validate())
	})
}

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

func TestSubscriberConfigValidateAdditionalBranches(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var cfg *SubscriberConfig
		err := cfg.Validate()
		require.EqualError(t, err, "subscriber config is nil")
	})

	t.Run("missing subjects", func(t *testing.T) {
		cfg := SubscriberConfig{
			URLs: []string{"nats://127.0.0.1:4222"},
		}
		err := cfg.Validate()
		require.EqualError(t, err, "subscriber subjects are required")
	})

	t.Run("invalid exhausted policy", func(t *testing.T) {
		cfg := SubscriberConfig{
			URLs:            []string{"nats://127.0.0.1:4222"},
			Subjects:        []string{"orders"},
			ExhaustedPolicy: ConsumeExhaustedPolicy("bad"),
		}
		err := cfg.Validate()
		require.EqualError(t, err, `unsupported consume exhausted policy "bad"`)
	})

	t.Run("invalid chain mode", func(t *testing.T) {
		cfg := SubscriberConfig{
			URLs:     []string{"nats://127.0.0.1:4222"},
			Subjects: []string{"orders"},
			SubjectHandlers: map[string]ConsumeSubjectHandlers{
				"orders": {Mode: ChainMode("bad")},
			},
		}
		err := cfg.Validate()
		require.EqualError(t, err, `consume subject "orders" uses unsupported chain mode "bad"`)
	})

	t.Run("default append mode", func(t *testing.T) {
		cfg := SubscriberConfig{
			URLs:     []string{"nats://127.0.0.1:4222"},
			Subjects: []string{"orders"},
			SubjectHandlers: map[string]ConsumeSubjectHandlers{
				"orders": {},
			},
		}
		require.NoError(t, cfg.Validate())
		require.Equal(t, ChainModeAppend, cfg.SubjectHandlers["orders"].Mode)
	})

	t.Run("conn satisfies dependency", func(t *testing.T) {
		cfg := SubscriberConfig{
			Conn:     &nats.Conn{},
			Subjects: []string{"orders"},
		}
		require.NoError(t, cfg.Validate())
	})
}

func TestRequesterConfigValidate(t *testing.T) {
	t.Run("defaults timeout", func(t *testing.T) {
		cfg := RequesterConfig{URLs: []string{" nats://127.0.0.1:4222 "}}
		require.NoError(t, cfg.Validate())
		require.Equal(t, []string{"nats://127.0.0.1:4222"}, cfg.URLs)
		require.Equal(t, defaultRequestTimeout, cfg.Timeout)
	})

	t.Run("conn satisfies dependency", func(t *testing.T) {
		cfg := RequesterConfig{Conn: &nats.Conn{}, Timeout: time.Second}
		require.NoError(t, cfg.Validate())
		require.Equal(t, time.Second, cfg.Timeout)
	})

	t.Run("nil config", func(t *testing.T) {
		var cfg *RequesterConfig
		err := cfg.Validate()
		require.EqualError(t, err, "requester config is nil")
	})

	t.Run("missing urls and conn", func(t *testing.T) {
		cfg := RequesterConfig{}
		err := cfg.Validate()
		require.EqualError(t, err, "requester URLs are required")
	})
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
	require.Equal(t, 1, cfg.ShardCount)
	require.Equal(t, DefaultConsumeShardQueueSize, cfg.ShardQueueSize)
}

func TestJetStreamConsumerConfigOrderedModeRequiresKeyExtractor(t *testing.T) {
	cfg := JetStreamConsumerConfig{
		URLs:       []string{"nats://127.0.0.1:4222"},
		Stream:     "ORDERS",
		Consumer:   "worker",
		ShardCount: 2,
	}

	err := cfg.Validate()
	require.EqualError(t, err, "ordered consume requires key extractor when shard count > 1")
}

func TestJetStreamConsumerConfigInvalidShardSettings(t *testing.T) {
	t.Run("negative shard count", func(t *testing.T) {
		cfg := JetStreamConsumerConfig{
			URLs:       []string{"nats://127.0.0.1:4222"},
			Stream:     "ORDERS",
			Consumer:   "worker",
			ShardCount: -1,
		}

		err := cfg.Validate()
		require.EqualError(t, err, "consume shard count must be > 0, got -1")
	})

	t.Run("negative shard queue size", func(t *testing.T) {
		cfg := JetStreamConsumerConfig{
			URLs:           []string{"nats://127.0.0.1:4222"},
			Stream:         "ORDERS",
			Consumer:       "worker",
			ShardQueueSize: -1,
		}

		err := cfg.Validate()
		require.EqualError(t, err, "consume shard queue size must be > 0, got -1")
	})
}

func TestJetStreamPublisherConfigValidate(t *testing.T) {
	t.Run("defaults and append mode", func(t *testing.T) {
		cfg := JetStreamPublisherConfig{
			URLs: []string{" nats://127.0.0.1:4222 "},
			SubjectHandlers: map[string]PublishSubjectHandlers{
				"orders": {},
			},
		}
		require.NoError(t, cfg.Validate())
		require.Equal(t, []string{"nats://127.0.0.1:4222"}, cfg.URLs)
		require.Equal(t, ChainModeAppend, cfg.SubjectHandlers["orders"].Mode)
	})

	t.Run("nil config", func(t *testing.T) {
		var cfg *JetStreamPublisherConfig
		err := cfg.Validate()
		require.EqualError(t, err, "jetstream publisher config is nil")
	})

	t.Run("missing deps", func(t *testing.T) {
		cfg := JetStreamPublisherConfig{}
		err := cfg.Validate()
		require.EqualError(t, err, "jetstream publisher URLs or injected JetStream are required")
	})
}
