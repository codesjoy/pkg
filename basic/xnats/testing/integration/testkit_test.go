//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	natsImage                 = "nats:2.10-alpine"
	natsStartupTimeout        = 2 * time.Minute
	natsShutdownTimeout       = 30 * time.Second
	defaultIntegrationTimeout = 60 * time.Second
)

var (
	integrationHarness *natsHarness
	nameCounter        atomic.Uint64
)

type natsHarness struct {
	container testcontainers.Container
	url       string
}

func TestMain(m *testing.M) {
	startupCtx, startupCancel := context.WithTimeout(context.Background(), natsStartupTimeout)
	defer startupCancel()

	harness, err := startNATSHarness(startupCtx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start nats harness: %v\n", err)
		os.Exit(1)
	}
	integrationHarness = harness

	exitCode := m.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), natsShutdownTimeout)
	defer shutdownCancel()
	if closeErr := integrationHarness.Close(shutdownCtx); closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop nats harness: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func startNATSHarness(ctx context.Context) (*natsHarness, error) {
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        natsImage,
			ExposedPorts: []string{"4222/tcp"},
			Cmd:          []string{"-js", "-sd", "/data"},
			WaitingFor:   wait.ForListeningPort("4222/tcp"),
		},
		Started: true,
	})
	if err != nil {
		return nil, fmt.Errorf("run nats container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(context.Background())
		return nil, fmt.Errorf("resolve nats host: %w", err)
	}
	port, err := container.MappedPort(ctx, "4222")
	if err != nil {
		_ = container.Terminate(context.Background())
		return nil, fmt.Errorf("resolve nats port: %w", err)
	}

	return &natsHarness{
		container: container,
		url:       fmt.Sprintf("nats://%s:%s", host, port.Port()),
	}, nil
}

func (h *natsHarness) Close(ctx context.Context) error {
	if h == nil || h.container == nil {
		return nil
	}
	return h.container.Terminate(ctx)
}

func mustURL(t *testing.T) string {
	t.Helper()
	require.NotNil(t, integrationHarness)
	require.NotEmpty(t, integrationHarness.url)
	return integrationHarness.url
}

func integrationContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), defaultIntegrationTimeout)
}

func newConn(t *testing.T) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(mustURL(t))
	require.NoError(t, err)
	return nc
}

func newJetStream(t *testing.T, nc *nats.Conn) jetstream.JetStream {
	t.Helper()
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	return js
}

func createStream(t *testing.T, js jetstream.JetStream, stream, subject string) {
	t.Helper()
	_, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     stream,
		Subjects: []string{subject},
	})
	require.NoError(t, err)
}

func waitForPushBound(
	t *testing.T,
	ctx context.Context,
	js nats.JetStreamContext,
	stream string,
	consumer string,
) *nats.ConsumerInfo {
	t.Helper()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		info, err := js.ConsumerInfo(stream, consumer)
		if err == nil && info != nil && info.PushBound {
			return info
		}

		select {
		case <-ctx.Done():
			if info != nil {
				t.Fatalf(
					"timed out waiting for push bind stream=%s consumer=%s push_bound=%t deliver_subject=%q",
					stream,
					consumer,
					info.PushBound,
					info.Config.DeliverSubject,
				)
			}
			t.Fatalf("timed out waiting for push bind stream=%s consumer=%s: %v", stream, consumer, err)
		case <-ticker.C:
		}
	}
}

func waitForPushBoundOrConsumeError(
	t *testing.T,
	ctx context.Context,
	js nats.JetStreamContext,
	stream string,
	consumer string,
	errCh <-chan error,
) *nats.ConsumerInfo {
	t.Helper()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-errCh:
			t.Fatalf("push consume exited before bind stream=%s consumer=%s: %v", stream, consumer, err)
		default:
		}

		info, err := js.ConsumerInfo(stream, consumer)
		if err == nil && info != nil && info.PushBound {
			return info
		}

		select {
		case err := <-errCh:
			t.Fatalf("push consume exited before bind stream=%s consumer=%s: %v", stream, consumer, err)
		case <-ctx.Done():
			if info != nil {
				t.Fatalf(
					"timed out waiting for push bind stream=%s consumer=%s push_bound=%t deliver_subject=%q",
					stream,
					consumer,
					info.PushBound,
					info.Config.DeliverSubject,
				)
			}
			t.Fatalf("timed out waiting for push bind stream=%s consumer=%s: %v", stream, consumer, err)
		case <-ticker.C:
		}
	}
}

func uniqueName(prefix string) string {
	cleanPrefix := strings.ToLower(strings.TrimSpace(prefix))
	if cleanPrefix == "" {
		cleanPrefix = "case"
	}
	cleanPrefix = strings.ReplaceAll(cleanPrefix, " ", "_")
	counter := nameCounter.Add(1)
	return fmt.Sprintf("xnats_it_%s_%d_%d", cleanPrefix, time.Now().UnixNano(), counter)
}
