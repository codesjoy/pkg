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

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

const (
	kafkaImage                  = "confluentinc/confluent-local:7.5.0"
	kafkaStartupTimeout         = 2 * time.Minute
	kafkaShutdownTimeout        = 30 * time.Second
	defaultIntegrationTimeout   = 120 * time.Second
	topicCreationRetryWait      = 300 * time.Millisecond
	topicCreationRetryMaxWindow = 30 * time.Second
)

var (
	integrationHarness *kafkaHarness
	nameCounter        atomic.Uint64
)

type kafkaHarness struct {
	container *tckafka.KafkaContainer
	brokers   []string
}

func TestMain(m *testing.M) {
	startupCtx, startupCancel := context.WithTimeout(context.Background(), kafkaStartupTimeout)
	defer startupCancel()

	harness, err := startKafkaHarness(startupCtx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start kafka harness: %v\n", err)
		os.Exit(1)
	}
	integrationHarness = harness

	exitCode := m.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), kafkaShutdownTimeout)
	defer shutdownCancel()
	if closeErr := integrationHarness.Close(shutdownCtx); closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop kafka harness: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func startKafkaHarness(ctx context.Context) (*kafkaHarness, error) {
	clusterID := uniqueKafkaName("cluster")
	container, err := tckafka.Run(ctx, kafkaImage, tckafka.WithClusterID(clusterID))
	if err != nil {
		return nil, fmt.Errorf("run kafka container: %w", err)
	}

	brokers, err := container.Brokers(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("resolve kafka brokers: %w", err)
	}
	if len(brokers) == 0 {
		_ = testcontainers.TerminateContainer(container)
		return nil, errors.New("kafka broker list is empty")
	}

	return &kafkaHarness{container: container, brokers: brokers}, nil
}

func (h *kafkaHarness) Close(_ context.Context) error {
	if h == nil || h.container == nil {
		return nil
	}
	return testcontainers.TerminateContainer(h.container)
}

func mustBrokers(t *testing.T) []string {
	t.Helper()
	require.NotNil(t, integrationHarness)
	require.NotEmpty(t, integrationHarness.brokers)
	brokers := make([]string, len(integrationHarness.brokers))
	copy(brokers, integrationHarness.brokers)
	return brokers
}

func integrationContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), defaultIntegrationTimeout)
}

func newConsumerSaramaConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Consumer.Return.Errors = true
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	return cfg
}

func newProducerSaramaConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Producer.Return.Successes = true
	return cfg
}

func createTopic(t *testing.T, topic string, partitions int32) {
	t.Helper()
	require.NotEmpty(t, topic)
	require.Greater(t, partitions, int32(0))

	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0

	admin, err := sarama.NewClusterAdmin(mustBrokers(t), cfg)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, admin.Close())
	}()

	detail := &sarama.TopicDetail{NumPartitions: partitions, ReplicationFactor: 1}
	deadline := time.Now().Add(topicCreationRetryMaxWindow)

	for {
		err = admin.CreateTopic(topic, detail, false)
		if err == nil || isTopicAlreadyExists(err) {
			return
		}
		if time.Now().After(deadline) {
			require.NoError(t, err)
		}
		time.Sleep(topicCreationRetryWait)
	}
}

func uniqueKafkaName(prefix string) string {
	cleanPrefix := strings.ToLower(strings.TrimSpace(prefix))
	if cleanPrefix == "" {
		cleanPrefix = "case"
	}
	cleanPrefix = strings.ReplaceAll(cleanPrefix, " ", "_")
	counter := nameCounter.Add(1)
	return fmt.Sprintf("xkafka_it_%s_%d_%d", cleanPrefix, time.Now().UnixNano(), counter)
}

func waitForError(t *testing.T, errCh <-chan error, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-errCh:
		return err
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for consume result after %s", timeout)
		return nil
	}
}

func isTopicAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sarama.ErrTopicAlreadyExists) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists")
}
