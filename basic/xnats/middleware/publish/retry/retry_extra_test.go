package retry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateConfigAndBackoff(t *testing.T) {
	t.Parallel()

	err := ValidateConfig(Config{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
		Multiplier:     2,
	})
	require.NoError(t, err)
	require.Equal(t, 2*time.Millisecond, Backoff(Config{
		InitialBackoff: time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
		Multiplier:     2,
	}, 3))
}
