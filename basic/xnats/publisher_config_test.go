package xnats

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
