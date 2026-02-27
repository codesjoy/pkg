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

//go:build integration_ha

package integration_ha

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	haTimeout       = 5 * time.Minute
	clusterBasePort = 22000
)

type sentinelHarness struct {
	options *redis.UniversalOptions
}

type clusterHarness struct {
	options *redis.UniversalOptions
}

func startSentinelHarness(t *testing.T) *sentinelHarness {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), haTimeout)
	defer cancel()

	sentinelConfig := strings.Join([]string{
		"port 26379",
		"bind 0.0.0.0",
		"protected-mode no",
		"dir /tmp",
		"sentinel monitor mymaster 127.0.0.1 6379 2",
		"sentinel down-after-milliseconds mymaster 5000",
		"sentinel failover-timeout mymaster 60000",
		"sentinel parallel-syncs mymaster 1",
	}, "\n") + "\n"

	req := testcontainers.ContainerRequest{
		Image: "redis:7",
		ExposedPorts: []string{
			"6379/tcp",
			"26379/tcp",
			"26380/tcp",
			"26381/tcp",
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(sentinelConfig),
				ContainerFilePath: "/tmp/sentinel-1.conf",
				FileMode:          0o644,
			},
			{
				Reader: strings.NewReader(
					strings.ReplaceAll(sentinelConfig, "26379", "26380"),
				),
				ContainerFilePath: "/tmp/sentinel-2.conf",
				FileMode:          0o644,
			},
			{
				Reader: strings.NewReader(
					strings.ReplaceAll(sentinelConfig, "26379", "26381"),
				),
				ContainerFilePath: "/tmp/sentinel-3.conf",
				FileMode:          0o644,
			},
		},
		Cmd: []string{
			"sh",
			"-c",
			"redis-server --bind 0.0.0.0 --protected-mode no --port 6379 & " +
				"redis-server /tmp/sentinel-1.conf --sentinel & " +
				"redis-server /tmp/sentinel-2.conf --sentinel & " +
				"redis-server /tmp/sentinel-3.conf --sentinel & wait",
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("6379/tcp"),
			wait.ForListeningPort("26379/tcp"),
			wait.ForListeningPort("26380/tcp"),
			wait.ForListeningPort("26381/tcp"),
		).WithStartupTimeout(2 * time.Minute),
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(ctr))
	})

	host, err := ctr.Host(ctx)
	require.NoError(t, err)

	masterPort, err := ctr.MappedPort(ctx, nat.Port("6379/tcp"))
	require.NoError(t, err)
	masterAddr := net.JoinHostPort(host, masterPort.Port())

	sentinelPorts := []nat.Port{"26379/tcp", "26380/tcp", "26381/tcp"}
	sentinelAddrs := make([]string, 0, len(sentinelPorts))
	for _, port := range sentinelPorts {
		mapped, mapErr := ctr.MappedPort(ctx, port)
		require.NoError(t, mapErr)
		sentinelAddrs = append(sentinelAddrs, net.JoinHostPort(host, mapped.Port()))
	}

	return &sentinelHarness{
		options: &redis.UniversalOptions{
			Addrs:      sentinelAddrs,
			MasterName: "mymaster",
			Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if addr == "127.0.0.1:6379" {
					addr = masterAddr
				}
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, network, addr)
			},
		},
	}
}

func startClusterHarness(t *testing.T) *clusterHarness {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), haTimeout)
	defer cancel()

	exposed := make([]string, 0, 6)
	for port := clusterBasePort; port <= clusterBasePort+5; port++ {
		exposed = append(exposed, fmt.Sprintf("%d/tcp", port))
	}

	req := testcontainers.ContainerRequest{
		Image:        "grokzen/redis-cluster:7.0.10",
		ExposedPorts: exposed,
		Env: map[string]string{
			"IP":                "127.0.0.1",
			"INITIAL_PORT":      strconv.Itoa(clusterBasePort),
			"MASTERS":           "3",
			"SLAVES_PER_MASTER": "1",
		},
		WaitingFor: wait.ForListeningPort(nat.Port(fmt.Sprintf("%d/tcp", clusterBasePort))).
			WithStartupTimeout(3 * time.Minute),
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(ctr))
	})

	time.Sleep(25 * time.Second)

	host, err := ctr.Host(ctx)
	require.NoError(t, err)

	addrMap := make(map[string]string, 6)
	seed := make([]string, 0, 3)
	for port := clusterBasePort; port <= clusterBasePort+5; port++ {
		mapped, mapErr := ctr.MappedPort(ctx, nat.Port(fmt.Sprintf("%d/tcp", port)))
		require.NoError(t, mapErr)
		mappedAddr := net.JoinHostPort(host, mapped.Port())
		addrMap[fmt.Sprintf("127.0.0.1:%d", port)] = mappedAddr
		if port < clusterBasePort+3 {
			seed = append(seed, mappedAddr)
		}
	}

	return &clusterHarness{
		options: &redis.UniversalOptions{
			Addrs: seed,
			Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
				for source, target := range addrMap {
					if strings.EqualFold(addr, source) {
						addr = target
						break
					}
				}
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, network, addr)
			},
		},
	}
}

func retryUntil(
	t *testing.T,
	attempts int,
	interval time.Duration,
	fn func() error,
) {
	t.Helper()

	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return
		}
		time.Sleep(interval)
	}
	require.NoError(t, err)
}
