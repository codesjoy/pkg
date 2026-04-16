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

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	debeziumgorm "github.com/codesjoy/pkg/basic/xevent/outbox/debezium/gorm"
	"github.com/codesjoy/pkg/basic/xevent/outbox/internal/shared"
	dockercontainer "github.com/docker/docker/api/types/container"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	postgresgorm "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	postgresImage         = "postgres:15-alpine"
	kafkaImage            = "confluentinc/confluent-local:7.5.0"
	debeziumConnectImage  = "quay.io/debezium/connect:3.4"
	harnessStartupTimeout = 8 * time.Minute
	harnessShutdownWait   = 45 * time.Second
	dbReadyTimeout        = 45 * time.Second
	connectReadyTimeout   = 2 * time.Minute
	connectorReadyTimeout = 90 * time.Second
	messageWaitTimeout    = 45 * time.Second

	testDBName     = "xevent_debezium_outbox_e2e"
	testDBUser     = "xevent"
	testDBPassword = "xevent"

	connectConfigTopic  = "xevent_debezium_connect_configs"
	connectOffsetsTopic = "xevent_debezium_connect_offsets"
	connectStatusTopic  = "xevent_debezium_connect_status"
)

var (
	e2eHarness  *harness
	nameCounter atomic.Uint64
)

type harness struct {
	network    *testcontainers.DockerNetwork
	postgres   *postgres.PostgresContainer
	kafka      *tckafka.KafkaContainer
	connect    testcontainers.Container
	tempDir    string
	brokers    []string
	dsn        string
	connectURL string
}

type txContextKey struct{}

type testEvent struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (*testEvent) EventType() string { return "order.created" }
func (e *testEvent) EventID() string { return e.ID }
func (e *testEvent) PartitionKey() string {
	return e.Key
}
func (*testEvent) Topic() string { return "" }
func (e *testEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(e)
}

func (e *testEvent) UnmarshalPayload(data []byte) error {
	return json.Unmarshal(data, e)
}

func TestMain(m *testing.M) {
	startupCtx, startupCancel := context.WithTimeout(context.Background(), harnessStartupTimeout)
	defer startupCancel()

	var err error
	e2eHarness, err = startHarness(startupCtx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start e2e harness: %v\n", err)
		os.Exit(1)
	}

	exitCode := m.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		harnessShutdownWait,
	)
	defer shutdownCancel()
	if closeErr := e2eHarness.Close(shutdownCtx); closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop e2e harness: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func TestCommittedEventReachesKafka(t *testing.T) {
	db := mustPostgresDB(t)
	resetTable(t, db)

	topic := uniqueName("orders")
	createTopic(t, topic, 1)

	connector := registerRunningConnector(t)
	t.Cleanup(func() {
		deleteConnector(t, connector)
	})

	store := newStore(t, db)
	var record *debezium.Record
	err := db.Transaction(func(tx *gorm.DB) error {
		ctx := withTransaction(context.Background(), tx)

		var appendErr error
		record, appendErr = debezium.AppendEvent(ctx, store, &testEvent{
			ID:    "evt_committed",
			Key:   "order-1",
			Value: "committed",
		}, debezium.AppendOptions{Topic: topic})
		return appendErr
	})
	require.NoError(t, err)
	require.NotNil(t, record)

	msg := consumeOneMessage(t, topic, messageWaitTimeout)
	require.Equal(t, topic, msg.Topic)
	require.Equal(t, []byte(record.PartitionKey), msg.Key)
	require.Equal(t, record.Payload, msg.Value)
	requireHeaderValue(t, msg, "id", record.ID)
	requireHeaderValue(t, msg, "x-event-type", record.EventType)
	requireHeaderValue(t, msg, "x-event-id", record.EventID)
}

func TestRollbackDoesNotPublishKafkaMessage(t *testing.T) {
	db := mustPostgresDB(t)
	resetTable(t, db)

	topic := uniqueName("orders")
	createTopic(t, topic, 1)

	connector := registerRunningConnector(t)
	t.Cleanup(func() {
		deleteConnector(t, connector)
	})

	store := newStore(t, db)
	err := db.Transaction(func(tx *gorm.DB) error {
		ctx := withTransaction(context.Background(), tx)
		_, appendErr := debezium.AppendEvent(ctx, store, &testEvent{
			ID:    "evt_rollback",
			Key:   "order-2",
			Value: "rolled-back",
		}, debezium.AppendOptions{Topic: topic})
		if appendErr != nil {
			return appendErr
		}
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)

	assertNoMessage(t, topic, 8*time.Second)
}

func TestTopicRoutingPublishesToConfiguredTopic(t *testing.T) {
	db := mustPostgresDB(t)
	resetTable(t, db)

	ordersTopic := uniqueName("orders")
	billingTopic := uniqueName("billing")
	createTopic(t, ordersTopic, 1)
	createTopic(t, billingTopic, 1)

	connector := registerRunningConnector(t)
	t.Cleanup(func() {
		deleteConnector(t, connector)
	})

	store := newStore(t, db)
	appendRecord := func(eventID, key, value, topic string) *debezium.Record {
		t.Helper()

		var record *debezium.Record
		err := db.Transaction(func(tx *gorm.DB) error {
			ctx := withTransaction(context.Background(), tx)

			var appendErr error
			record, appendErr = debezium.AppendEvent(ctx, store, &testEvent{
				ID:    eventID,
				Key:   key,
				Value: value,
			}, debezium.AppendOptions{Topic: topic})
			return appendErr
		})
		require.NoError(t, err)
		require.NotNil(t, record)
		return record
	}

	ordersRecord := appendRecord("evt_orders", "order-3", "orders", ordersTopic)
	billingRecord := appendRecord("evt_billing", "bill-9", "billing", billingTopic)

	ordersMsg := consumeOneMessage(t, ordersTopic, messageWaitTimeout)
	billingMsg := consumeOneMessage(t, billingTopic, messageWaitTimeout)

	require.Equal(t, ordersTopic, ordersMsg.Topic)
	require.Equal(t, []byte(ordersRecord.PartitionKey), ordersMsg.Key)
	require.Equal(t, ordersRecord.Payload, ordersMsg.Value)

	require.Equal(t, billingTopic, billingMsg.Topic)
	require.Equal(t, []byte(billingRecord.PartitionKey), billingMsg.Key)
	require.Equal(t, billingRecord.Payload, billingMsg.Value)
}

func startHarness(ctx context.Context) (*harness, error) {
	tempDir, err := os.MkdirTemp("", "xevent-debezium-e2e-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	cleanupTempDir := func() {
		_ = os.RemoveAll(tempDir)
	}

	replicationScript, err := writeReplicationInitScript(tempDir)
	if err != nil {
		cleanupTempDir()
		return nil, fmt.Errorf("write replication init script: %w", err)
	}

	net, err := tcnetwork.New(ctx, tcnetwork.WithAttachable())
	if err != nil {
		cleanupTempDir()
		return nil, fmt.Errorf("create network: %w", err)
	}

	closeNetwork := func() {
		_ = net.Remove(context.Background())
	}

	kafkaContainer, err := tckafka.Run(
		ctx,
		kafkaImage,
		tckafka.WithClusterID(uniqueName("cluster")),
		tcnetwork.WithNetwork([]string{"kafka"}, net),
		testcontainers.WithConfigModifier(func(cfg *dockercontainer.Config) {
			cfg.Hostname = "kafka"
		}),
	)
	if err != nil {
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("start kafka: %w", err)
	}

	brokers, err := kafkaContainer.Brokers(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("resolve kafka brokers: %w", err)
	}
	if len(brokers) == 0 {
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, errors.New("kafka brokers list is empty")
	}

	if err := createCompactedTopicWithContext(ctx, brokers, connectConfigTopic, 1); err != nil {
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("create connect config topic: %w", err)
	}
	if err := createCompactedTopicWithContext(ctx, brokers, connectOffsetsTopic, 1); err != nil {
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("create connect offsets topic: %w", err)
	}
	if err := createCompactedTopicWithContext(ctx, brokers, connectStatusTopic, 1); err != nil {
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("create connect status topic: %w", err)
	}

	postgresContainer, err := postgres.Run(
		ctx,
		postgresImage,
		postgres.WithDatabase(testDBName),
		postgres.WithUsername(testDBUser),
		postgres.WithPassword(testDBPassword),
		postgres.WithInitScripts(replicationScript),
		tcnetwork.WithNetwork([]string{"postgres"}, net),
		testcontainers.WithConfigModifier(func(cfg *dockercontainer.Config) {
			cfg.Hostname = "postgres"
		}),
		testcontainers.WithCmd(
			"postgres",
			"-c", "fsync=off",
			"-c", "listen_addresses=*",
			"-c", "wal_level=logical",
			"-c", "max_wal_senders=8",
			"-c", "max_replication_slots=8",
		),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(harnessStartupTimeout),
		),
	)
	if err != nil {
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("start postgres: %w", err)
	}

	dsn, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(postgresContainer)
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("resolve postgres dsn: %w", err)
	}
	if err := waitForDB(ctx, "pgx", dsn); err != nil {
		_ = testcontainers.TerminateContainer(postgresContainer)
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("wait for postgres: %w", err)
	}

	connectContainer, err := testcontainers.Run(
		ctx,
		debeziumConnectImage,
		testcontainers.WithExposedPorts("8083/tcp"),
		testcontainers.WithEnv(map[string]string{
			"BOOTSTRAP_SERVERS":                 "kafka:9092",
			"GROUP_ID":                          "1",
			"CONFIG_STORAGE_TOPIC":              connectConfigTopic,
			"OFFSET_STORAGE_TOPIC":              connectOffsetsTopic,
			"STATUS_STORAGE_TOPIC":              connectStatusTopic,
			"CONFIG_STORAGE_REPLICATION_FACTOR": "1",
			"OFFSET_STORAGE_REPLICATION_FACTOR": "1",
			"STATUS_STORAGE_REPLICATION_FACTOR": "1",
			"KEY_CONVERTER":                     "org.apache.kafka.connect.storage.StringConverter",
			"VALUE_CONVERTER":                   "org.apache.kafka.connect.storage.StringConverter",
			"REST_ADVERTISED_HOST_NAME":         "connect",
			"REST_PORT":                         "8083",
		}),
		tcnetwork.WithNetwork([]string{"connect"}, net),
		testcontainers.WithConfigModifier(func(cfg *dockercontainer.Config) {
			cfg.Hostname = "connect"
		}),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/connectors").
				WithPort("8083/tcp").
				WithStartupTimeout(connectReadyTimeout).
				WithForcedIPv4LocalHost(),
		),
	)
	if err != nil {
		_ = testcontainers.TerminateContainer(postgresContainer)
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("start debezium connect: %w", err)
	}

	connectHost, err := connectContainer.Host(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(connectContainer)
		_ = testcontainers.TerminateContainer(postgresContainer)
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("resolve connect host: %w", err)
	}
	connectPort, err := connectContainer.MappedPort(ctx, "8083/tcp")
	if err != nil {
		_ = testcontainers.TerminateContainer(connectContainer)
		_ = testcontainers.TerminateContainer(postgresContainer)
		_ = testcontainers.TerminateContainer(kafkaContainer)
		closeNetwork()
		cleanupTempDir()
		return nil, fmt.Errorf("resolve connect port: %w", err)
	}

	return &harness{
		network:    net,
		postgres:   postgresContainer,
		kafka:      kafkaContainer,
		connect:    connectContainer,
		tempDir:    tempDir,
		brokers:    brokers,
		dsn:        dsn,
		connectURL: fmt.Sprintf("http://%s:%s", connectHost, connectPort.Port()),
	}, nil
}

func (h *harness) Close(ctx context.Context) error {
	var closeErr error
	if h == nil {
		return nil
	}
	if h.connect != nil {
		closeErr = errors.Join(closeErr, testcontainers.TerminateContainer(h.connect))
	}
	if h.postgres != nil {
		closeErr = errors.Join(closeErr, testcontainers.TerminateContainer(h.postgres))
	}
	if h.kafka != nil {
		closeErr = errors.Join(closeErr, testcontainers.TerminateContainer(h.kafka))
	}
	if h.network != nil {
		closeErr = errors.Join(closeErr, h.network.Remove(ctx))
	}
	if h.tempDir != "" {
		closeErr = errors.Join(closeErr, os.RemoveAll(h.tempDir))
	}
	return closeErr
}

func writeReplicationInitScript(dir string) (string, error) {
	scriptPath := filepath.Join(dir, "001-enable-replication.sh")
	script := `#!/bin/sh
set -eu
cat <<'EOF' >> "$PGDATA/pg_hba.conf"
host all all 0.0.0.0/0 scram-sha-256
host all all ::0/0 scram-sha-256
host replication all 0.0.0.0/0 scram-sha-256
host replication all ::0/0 scram-sha-256
EOF
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func newStore(t *testing.T, db *gorm.DB) *debeziumgorm.GORMStore {
	t.Helper()

	store, err := debeziumgorm.NewGORMStore(debeziumgorm.GORMStoreConfig{
		DB:                 db,
		SessionFromContext: transactionFromContext,
	})
	require.NoError(t, err)
	return store
}

func withTransaction(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func transactionFromContext(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(txContextKey{}).(*gorm.DB)
	return tx
}

func mustPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NotNil(t, e2eHarness)

	db, err := gorm.Open(postgresgorm.Open(e2eHarness.dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), dbReadyTimeout)
	defer cancel()
	require.NoError(t, sqlDB.PingContext(ctx))

	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	return db
}

func waitForDB(ctx context.Context, driverName, dsn string) error {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	deadline := time.Now().Add(dbReadyTimeout)
	consecutiveSuccess := 0
	for {
		if err := db.PingContext(ctx); err == nil {
			consecutiveSuccess++
			if consecutiveSuccess >= 3 {
				return nil
			}
		} else {
			consecutiveSuccess = 0
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("database %s did not become ready in time", driverName)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func resetTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Migrator().DropTable(&shared.DBRecord{}))
	require.NoError(t, db.AutoMigrate(&shared.DBRecord{}))
}

func registerRunningConnector(t *testing.T) string {
	t.Helper()
	require.NotNil(t, e2eHarness)

	connectorName := uniqueName("connector")
	historyTopic := uniqueName("schema_history")
	require.NoError(t, createCompactedTopicWithContext(context.Background(), e2eHarness.brokers, historyTopic, 1))

	cfg := map[string]string{
		"connector.class":             "io.debezium.connector.postgresql.PostgresConnector",
		"plugin.name":                 "pgoutput",
		"database.hostname":           "postgres",
		"database.port":               "5432",
		"database.user":               testDBUser,
		"database.password":           testDBPassword,
		"database.dbname":             testDBName,
		"database.sslmode":            "disable",
		"topic.prefix":                uniqueName("topic_prefix"),
		"schema.include.list":         "public",
		"table.include.list":          "public." + (debezium.Record{}).TableName(),
		"publication.autocreate.mode": "filtered",
		"publication.name":            uniqueName("publication"),
		"slot.name":                   uniqueName("slot"),
		"slot.drop.on.stop":           "true",
		"snapshot.mode":               "no_data",
		"tombstones.on.delete":        "false",
		"schema.history.internal.kafka.bootstrap.servers":        "kafka:9092",
		"schema.history.internal.kafka.topic":                    historyTopic,
		"transforms":                                             "outbox",
		"transforms.outbox.type":                                 "io.debezium.transforms.outbox.EventRouter",
		"transforms.outbox.table.op.invalid.behavior":            "fatal",
		"transforms.outbox.table.field.event.id":                 "message_id",
		"transforms.outbox.route.by.field":                       "topic",
		"transforms.outbox.route.topic.replacement":              "${routedByValue}",
		"transforms.outbox.table.field.event.key":                "partition_key",
		"transforms.outbox.table.field.event.payload":            "payload",
		"transforms.outbox.table.fields.additional.placement":    "event_type:header:x-event-type,event_id:header:x-event-id",
		"value.converter":                                        "io.debezium.converters.BinaryDataConverter",
		"value.converter.delegate.converter.type":                "org.apache.kafka.connect.json.JsonConverter",
		"value.converter.delegate.converter.type.schemas.enable": "false",
	}

	registerConnector(t, connectorName, cfg)
	waitForConnectorRunning(t, connectorName)
	return connectorName
}

func registerConnector(t *testing.T, name string, cfg map[string]string) {
	t.Helper()
	require.NotNil(t, e2eHarness)

	body := map[string]any{
		"name":   name,
		"config": cfg,
	}
	data, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, e2eHarness.connectURL+"/connectors", bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(messageWaitTimeout).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("register connector %s failed: status=%d body=%s", name, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func waitForConnectorRunning(t *testing.T, name string) {
	t.Helper()
	require.NotNil(t, e2eHarness)

	deadline := time.Now().Add(connectorReadyTimeout)
	for {
		status, err := connectorStatus(name)
		if err == nil && status.Connector.State == "RUNNING" && len(status.Tasks) > 0 {
			allRunning := true
			for _, task := range status.Tasks {
				if task.State != "RUNNING" {
					allRunning = false
					if task.State == "FAILED" {
						t.Fatalf("connector %s task failed: %s", name, task.Trace)
					}
				}
			}
			if allRunning {
				return
			}
		}
		if err == nil && status.Connector.State == "FAILED" {
			t.Fatalf("connector %s failed: %s", name, status.Connector.Trace)
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("timed out waiting for connector %s: %v", name, err)
			}
			t.Fatalf("timed out waiting for connector %s to be running: connector=%s tasks=%v", name, status.Connector.State, status.Tasks)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func deleteConnector(t *testing.T, name string) {
	t.Helper()
	require.NotNil(t, e2eHarness)

	req, err := http.NewRequest(http.MethodDelete, e2eHarness.connectURL+"/connectors/"+name, nil)
	require.NoError(t, err)

	resp, err := httpClient(30 * time.Second).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("delete connector %s failed: status=%d body=%s", name, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

type connectorStatusResponse struct {
	Connector struct {
		State string `json:"state"`
		Trace string `json:"trace"`
	} `json:"connector"`
	Tasks []connectorTaskStatus `json:"tasks"`
}

type connectorTaskStatus struct {
	ID    int    `json:"id"`
	State string `json:"state"`
	Trace string `json:"trace"`
}

func connectorStatus(name string) (connectorStatusResponse, error) {
	var status connectorStatusResponse
	req, err := http.NewRequest(http.MethodGet, e2eHarness.connectURL+"/connectors/"+name+"/status", nil)
	if err != nil {
		return status, err
	}

	resp, err := httpClient(15 * time.Second).Do(req)
	if err != nil {
		return status, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return status, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return status, err
	}
	return status, nil
}

func createTopic(t *testing.T, topic string, partitions int32) {
	t.Helper()
	require.NotNil(t, e2eHarness)
	require.NoError(t, createTopicWithContext(context.Background(), e2eHarness.brokers, topic, partitions))
}

func createTopicWithContext(ctx context.Context, brokers []string, topic string, partitions int32) error {
	return createTopicWithConfig(ctx, brokers, topic, partitions, nil)
}

func createCompactedTopicWithContext(ctx context.Context, brokers []string, topic string, partitions int32) error {
	value := "compact"
	return createTopicWithConfig(ctx, brokers, topic, partitions, map[string]*string{
		"cleanup.policy": &value,
	})
}

func createTopicWithConfig(
	ctx context.Context,
	brokers []string,
	topic string,
	partitions int32,
	configEntries map[string]*string,
) error {
	if topic == "" {
		return errors.New("topic is required")
	}
	if partitions <= 0 {
		return fmt.Errorf("partitions must be > 0, got %d", partitions)
	}

	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0

	admin, err := sarama.NewClusterAdmin(brokers, cfg)
	if err != nil {
		return err
	}
	defer admin.Close()

	detail := &sarama.TopicDetail{
		NumPartitions:     partitions,
		ReplicationFactor: 1,
		ConfigEntries:     configEntries,
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		err = admin.CreateTopic(topic, detail, false)
		if err == nil || isTopicAlreadyExists(err) {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func consumeOneMessage(t *testing.T, topic string, timeout time.Duration) *sarama.ConsumerMessage {
	t.Helper()
	require.NotNil(t, e2eHarness)

	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Consumer.Return.Errors = true
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer, err := sarama.NewConsumer(e2eHarness.brokers, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, consumer.Close())
	})

	partitionConsumer, err := consumer.ConsumePartition(topic, 0, sarama.OffsetOldest)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, partitionConsumer.Close())
	})

	select {
	case msg := <-partitionConsumer.Messages():
		return msg
	case err := <-partitionConsumer.Errors():
		t.Fatalf("consume topic %s failed: %v", topic, err)
		return nil
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for kafka message on topic %s after %s", topic, timeout)
		return nil
	}
}

func assertNoMessage(t *testing.T, topic string, timeout time.Duration) {
	t.Helper()
	require.NotNil(t, e2eHarness)

	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Consumer.Return.Errors = true
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer, err := sarama.NewConsumer(e2eHarness.brokers, cfg)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	partitionConsumer, err := consumer.ConsumePartition(topic, 0, sarama.OffsetOldest)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, partitionConsumer.Close())
	}()

	select {
	case msg := <-partitionConsumer.Messages():
		t.Fatalf("expected no kafka message on topic %s, got key=%q value=%q", topic, string(msg.Key), string(msg.Value))
	case err := <-partitionConsumer.Errors():
		t.Fatalf("consume topic %s failed: %v", topic, err)
	case <-time.After(timeout):
	}
}

func requireHeaderValue(t *testing.T, msg *sarama.ConsumerMessage, key, want string) {
	t.Helper()
	for _, header := range msg.Headers {
		if string(header.Key) == key {
			require.Equal(t, want, string(header.Value))
			return
		}
	}
	t.Fatalf("expected kafka header %q to be present", key)
}

func httpClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func uniqueName(prefix string) string {
	cleanPrefix := strings.ToLower(strings.TrimSpace(prefix))
	if cleanPrefix == "" {
		cleanPrefix = "case"
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_", ".", "_")
	cleanPrefix = replacer.Replace(cleanPrefix)
	counter := nameCounter.Add(1)
	return fmt.Sprintf("xev_%s_%x_%x", cleanPrefix, time.Now().UnixNano(), counter)
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
