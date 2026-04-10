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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	_ "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

type testServer struct {
	url    string
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (s *testServer) ClientURL() string {
	return s.url
}

var (
	natsServerBinaryOnce sync.Once
	natsServerBinaryPath string
	natsServerBinaryErr  error
	natsListenPattern    = regexp.MustCompile(`127\.0\.0\.1:(\d+)`)
)

func newTestServer(t *testing.T) *testServer {
	return newTestServerWithArgs(t)
}

func newTestServerWithArgs(t *testing.T, args ...string) *testServer {
	t.Helper()

	stderr := &bytes.Buffer{}
	baseArgs := []string{"-a", "127.0.0.1", "-p", "-1"}
	baseArgs = append(baseArgs, args...)
	// #nosec G204 -- test starts a locally built nats-server with controlled arguments.
	cmd := exec.Command(natsServerBinary(t), baseArgs...)
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/go-build")
	require.NoError(t, cmd.Start())

	srv := &testServer{cmd: cmd, stderr: stderr}
	require.Eventually(t, func() bool {
		if srv.url == "" {
			srv.url = parseServerURL(stderr.String())
		}
		if srv.url == "" {
			return false
		}
		nc, err := nats.Connect(srv.url, nats.Timeout(200*time.Millisecond))
		if err != nil {
			return false
		}
		nc.Close()
		return true
	}, 10*time.Second, 100*time.Millisecond, stderr.String())

	t.Cleanup(func() {
		stopTestServer(srv)
	})
	return srv
}

func newTestConn(t *testing.T, srv *testServer) *nats.Conn {
	t.Helper()

	require.NotNil(t, srv)
	nc, err := nats.Connect(srv.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}

func natsServerBinary(t *testing.T) string {
	t.Helper()

	natsServerBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "xnats-nats-server-*")
		if err != nil {
			natsServerBinaryErr = err
			return
		}
		natsServerBinaryPath = filepath.Join(dir, "nats-server")
		// #nosec G204 -- test builds a fixed helper binary with constant go build arguments.
		cmd := exec.Command(
			"go",
			"build",
			"-o",
			natsServerBinaryPath,
			"github.com/nats-io/nats-server/v2",
		)
		cmd.Env = append(os.Environ(), "GOCACHE=/tmp/go-build")
		output, err := cmd.CombinedOutput()
		if err != nil {
			natsServerBinaryErr = fmt.Errorf("build nats-server: %w: %s", err, string(output))
			return
		}
	})

	require.NoError(t, natsServerBinaryErr)
	return natsServerBinaryPath
}

func stopTestServer(srv *testServer) {
	if srv == nil || srv.cmd == nil || srv.cmd.Process == nil {
		return
	}

	_ = srv.cmd.Process.Signal(os.Interrupt)

	done := make(chan struct{})
	go func() {
		_, _ = srv.cmd.Process.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = srv.cmd.Process.Kill()
		<-done
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			nc, err := nats.Connect(srv.url, nats.Timeout(100*time.Millisecond))
			if err != nil {
				return
			}
			nc.Close()
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func parseServerURL(logOutput string) string {
	match := natsListenPattern.FindStringSubmatch(logOutput)
	if len(match) != 2 {
		return ""
	}
	return fmt.Sprintf("nats://127.0.0.1:%s", match[1])
}

func TestNewPublisherAndPublish(t *testing.T) {
	srv := newTestServer(t)
	subConn := newTestConn(t, srv)
	sub, err := subConn.SubscribeSync("orders.created")
	require.NoError(t, err)
	require.NoError(t, subConn.Flush())

	publisher, err := NewPublisher(PublisherConfig{
		URLs:           []string{srv.ClientURL()},
		DefaultSubject: "orders.created",
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	result, err := publisher.Publish(context.Background(), &publish.Message{Data: []byte("a")})
	require.NoError(t, err)
	require.Equal(t, "orders.created", result.Subject)

	msg, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, []byte("a"), msg.Data)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = publisher.Publish(ctx, &publish.Message{Subject: "orders.created", Data: []byte("b")})
	require.NoError(t, err)

	msg, err = sub.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, []byte("b"), msg.Data)
}

func TestPublisherPublishBatch(t *testing.T) {
	srv := newTestServer(t)
	subConn := newTestConn(t, srv)
	sub, err := subConn.SubscribeSync("batch.subject")
	require.NoError(t, err)
	require.NoError(t, subConn.Flush())

	publisher, err := NewPublisher(PublisherConfig{URLs: []string{srv.ClientURL()}})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	results, err := publisher.PublishBatch(
		context.Background(),
		&publish.Message{Subject: "batch.subject", Data: []byte("one")},
		&publish.Message{Subject: "batch.subject", Data: []byte("two")},
	)
	require.NoError(t, err)
	require.Len(t, results, 2)

	gotOne, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	gotTwo, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, "one", string(gotOne.Data))
	require.Equal(t, "two", string(gotTwo.Data))

	results, err = publisher.PublishBatch(context.Background())
	require.NoError(t, err)
	require.Nil(t, results)
}

func TestPublisherPublishBatchFailsWithIndex(t *testing.T) {
	srv := newTestServer(t)
	subConn := newTestConn(t, srv)
	sub, err := subConn.SubscribeSync("orders")
	require.NoError(t, err)
	require.NoError(t, subConn.Flush())

	publisher, err := NewPublisher(PublisherConfig{
		URLs:           []string{srv.ClientURL()},
		DefaultSubject: "orders",
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	results, err := publisher.PublishBatch(
		context.Background(),
		&publish.Message{Data: []byte("ok")},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "publish batch index 1")
	require.Len(t, results, 2)
	require.NotNil(t, results[0])
	require.Nil(t, results[1])

	msg, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, "ok", string(msg.Data))
}

func TestPublisherPublishBatchReportReturnsPerItemResults(t *testing.T) {
	srv := newTestServer(t)
	subConn := newTestConn(t, srv)
	sub, err := subConn.SubscribeSync("orders")
	require.NoError(t, err)
	require.NoError(t, subConn.Flush())

	publisher, err := NewPublisher(PublisherConfig{
		URLs:           []string{srv.ClientURL()},
		DefaultSubject: "orders",
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	results, err := publisher.PublishBatchReport(
		context.Background(),
		&publish.Message{Data: []byte("ok-1")},
		nil,
		&publish.Message{Data: []byte("ok-2")},
	)
	require.NoError(t, err)
	require.Len(t, results, 3)
	require.NotNil(t, results[0].Result)
	require.NoError(t, results[0].Err)
	require.Nil(t, results[1].Result)
	require.ErrorIs(t, results[1].Err, ErrNilPublishMessage)
	require.NotNil(t, results[2].Result)
	require.NoError(t, results[2].Err)

	gotOne, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	gotTwo, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, "ok-1", string(gotOne.Data))
	require.Equal(t, "ok-2", string(gotTwo.Data))
}

func TestPublisherPublishBatchReportCanceledBeforeStart(t *testing.T) {
	srv := newTestServer(t)
	publisher, err := NewPublisher(PublisherConfig{
		URLs: []string{srv.ClientURL()},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, publisher.Close())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := publisher.PublishBatchReport(
		ctx,
		&publish.Message{Subject: "orders", Data: []byte("a")},
	)
	require.Nil(t, results)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPublisherErrorPathsAndClose(t *testing.T) {
	t.Parallel()

	var nilPublisher *Publisher
	_, err := nilPublisher.Publish(context.Background(), &publish.Message{})
	require.EqualError(t, err, "publisher is nil")
	require.NoError(t, nilPublisher.Close())

	srv := newTestServer(t)
	publisher, err := NewPublisher(PublisherConfig{URLs: []string{srv.ClientURL()}})
	require.NoError(t, err)
	require.NoError(t, publisher.Close())
	require.NoError(t, publisher.Close())

	_, err = publisher.Publish(context.Background(), &publish.Message{Subject: "orders"})
	require.ErrorIs(t, err, ErrPublisherClosed)
}

func TestPublisherPrepareMessageAndHelpers(t *testing.T) {
	t.Parallel()

	publisher := &Publisher{cfg: PublisherConfig{DefaultSubject: "orders.created"}}

	prepared, err := publisher.prepareMessage(&publish.Message{
		Reply: "reply.to",
		Data:  []byte("payload"),
		Header: nats.Header{
			"X-Test": {"value"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "orders.created", prepared.Subject)

	prepared.Data[0] = 'P'
	prepared.Header.Set("X-Test", "changed")
	original := &publish.Message{
		Subject: "subject",
		Reply:   "reply",
		Data:    []byte("body"),
		Header:  nats.Header{"A": {"1"}},
	}
	cloned := clonePublishMessage(original)
	raw := toNATSMessage(original)
	headers := cloneHeader(original.Header)
	original.Data[0] = 'B'
	original.Header.Set("A", "2")
	require.Equal(t, "body", string(cloned.Data))
	require.Equal(t, "body", string(raw.Data))
	require.Equal(t, []string{"1"}, headers["A"])

	_, err = publisher.prepareMessage(nil)
	require.ErrorIs(t, err, ErrNilPublishMessage)

	_, err = (&Publisher{}).prepareMessage(&publish.Message{})
	require.ErrorIs(t, err, ErrPublishSubjectRequired)

	_, err = publisher.send(context.Background(), nil)
	require.ErrorIs(t, err, ErrNilPublishMessage)

	_, err = publisher.send(context.Background(), &publish.MessageContext{})
	require.ErrorIs(t, err, ErrNilPublishMessage)
}

func TestNewPublisherWithInjectedConn(t *testing.T) {
	srv := newTestServer(t)
	nc := newTestConn(t, srv)

	publisher, err := NewPublisher(PublisherConfig{Conn: nc})
	require.NoError(t, err)
	require.False(t, publisher.ownConn)
	require.NoError(t, publisher.Close())
}

func TestNewRequesterAndRequest(t *testing.T) {
	srv := newTestServer(t)
	nc := newTestConn(t, srv)
	_, err := nc.Subscribe("rpc.echo", func(msg *nats.Msg) {
		_ = msg.Respond([]byte("pong"))
	})
	require.NoError(t, err)
	require.NoError(t, nc.Flush())

	requester, err := NewRequester(RequesterConfig{
		URLs:    []string{srv.ClientURL()},
		Timeout: time.Second,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, requester.Close())
	}()

	reply, err := requester.Request(context.Background(), &publish.Message{
		Subject: "rpc.echo",
		Data:    []byte("ping"),
	})
	require.NoError(t, err)
	require.Equal(t, "pong", string(reply.Data))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reply, err = requester.RequestMsg(ctx, &nats.Msg{Subject: "rpc.echo", Data: []byte("ping-2")})
	require.NoError(t, err)
	require.Equal(t, "pong", string(reply.Data))
}

func TestRequesterRequestMsgUsesDefaultTimeout(t *testing.T) {
	srv := newTestServer(t)
	requester, err := NewRequester(RequesterConfig{
		URLs:    []string{srv.ClientURL()},
		Timeout: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, requester.Close())
	}()

	start := time.Now()
	_, err = requester.RequestMsg(context.Background(), &nats.Msg{Subject: "rpc.timeout"})
	require.Error(t, err)
	require.Less(t, time.Since(start), time.Second)
}

func TestRequesterErrorPathsAndClose(t *testing.T) {
	t.Parallel()

	var nilRequester *Requester
	_, err := nilRequester.Request(context.Background(), &publish.Message{})
	require.EqualError(t, err, "requester is nil")
	_, err = nilRequester.RequestMsg(context.Background(), &nats.Msg{})
	require.EqualError(t, err, "requester is nil")
	require.NoError(t, nilRequester.Close())

	requester := &Requester{conn: &nats.Conn{}}
	_, err = requester.Request(context.Background(), nil)
	require.ErrorIs(t, err, ErrNilPublishMessage)

	_, err = requester.Request(context.Background(), &publish.Message{})
	require.ErrorIs(t, err, ErrPublishSubjectRequired)

	require.NoError(t, requester.Close())
	require.NoError(t, requester.Close())

	_, err = requester.Request(context.Background(), &publish.Message{Subject: "orders"})
	require.ErrorIs(t, err, ErrRequesterClosed)
	_, err = requester.RequestMsg(context.Background(), &nats.Msg{Subject: "orders"})
	require.ErrorIs(t, err, ErrRequesterClosed)
}

func TestNewRequesterWithInjectedConn(t *testing.T) {
	srv := newTestServer(t)
	nc := newTestConn(t, srv)

	requester, err := NewRequester(RequesterConfig{Conn: nc})
	require.NoError(t, err)
	require.False(t, requester.ownConn)
	require.NoError(t, requester.Close())
}

func TestNewSubscriberAndConsume(t *testing.T) {
	srv := newTestServer(t)
	pubConn := newTestConn(t, srv)

	subscriber, err := NewSubscriber(SubscriberConfig{
		URLs:     []string{srv.ClientURL()},
		Subjects: []string{"orders.created"},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, subscriber.Close())
	}()

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- subscriber.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			received <- string(msg.Message.Data)
			cancel()
			return nil
		})
	}()

	require.Eventually(t, func() bool {
		subscriber.mu.Lock()
		defer subscriber.mu.Unlock()
		return subscriber.active
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, pubConn.Publish("orders.created", []byte("payload")))
	require.Equal(t, "payload", <-received)
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestSubscriberConsumeQueueGroupAndHandlerError(t *testing.T) {
	srv := newTestServer(t)
	pubConn := newTestConn(t, srv)

	subscriber, err := NewSubscriber(SubscriberConfig{
		URLs:       []string{srv.ClientURL()},
		Subjects:   []string{"orders.failed"},
		QueueGroup: "workers",
		RetryConfig: RetryConfig{
			MaxRetries:     0,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
			Multiplier:     1,
		},
		ExhaustedPolicy: ConsumeExhaustedPolicyStop,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, subscriber.Close())
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- subscriber.Consume(context.Background(), func(_ context.Context, msg *consume.MessageContext) error {
			require.Equal(t, consume.TransportCore, msg.Transport)
			return errors.New("boom")
		})
	}()

	require.Eventually(t, func() bool {
		subscriber.mu.Lock()
		defer subscriber.mu.Unlock()
		return subscriber.active
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, pubConn.Publish("orders.failed", []byte("payload")))
	require.EqualError(t, <-errCh, "handle message exhausted retries: boom")
}

func TestSubscriberConsumeErrorPathsAndClose(t *testing.T) {
	t.Parallel()

	var nilSubscriber *Subscriber
	err := nilSubscriber.Consume(
		context.Background(),
		func(context.Context, *consume.MessageContext) error { return nil },
	)
	require.EqualError(t, err, "subscriber is nil")
	require.NoError(t, nilSubscriber.Close())

	subscriber := &Subscriber{
		cfg:      SubscriberConfig{Subjects: []string{"orders"}},
		conn:     &nats.Conn{},
		closedCh: make(chan struct{}),
	}
	err = subscriber.Consume(context.Background(), nil)
	require.ErrorIs(t, err, consume.ErrNilHandlerFunc)

	require.NoError(t, subscriber.Close())
	require.NoError(t, subscriber.Close())
	err = subscriber.Consume(
		context.Background(),
		func(context.Context, *consume.MessageContext) error { return nil },
	)
	require.ErrorIs(t, err, ErrSubscriberClosed)
}

func TestSubscriberConsumeActive(t *testing.T) {
	srv := newTestServer(t)
	pubConn := newTestConn(t, srv)
	subscriber, err := NewSubscriber(SubscriberConfig{
		URLs:     []string{srv.ClientURL()},
		Subjects: []string{"orders.active"},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, subscriber.Close())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- subscriber.Consume(ctx, func(_ context.Context, _ *consume.MessageContext) error {
			<-ctx.Done()
			return nil
		})
	}()

	require.Eventually(t, func() bool {
		subscriber.mu.Lock()
		defer subscriber.mu.Unlock()
		return subscriber.active
	}, time.Second, 10*time.Millisecond)

	err = subscriber.Consume(
		context.Background(),
		func(context.Context, *consume.MessageContext) error { return nil },
	)
	require.ErrorIs(t, err, ErrSubscriberActive)

	cancel()
	require.NoError(t, pubConn.Publish("orders.active", []byte("wake")))
	require.ErrorIs(t, <-errCh, context.Canceled)
}
