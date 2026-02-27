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

package xnet

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func TestFilterAndSortAddrs(t *testing.T) {
	t.Parallel()

	raw := parseAddrs(
		t,
		"0.0.0.0",
		"::",
		"::ffff:10.0.0.1",
		"10.0.0.1",
		"::1",
		"fe80::1",
		"2001:db8::1",
		"2001:db8::1",
	)

	tests := []struct {
		name string
		opts LocalAddrOptions
		want []netip.Addr
	}{
		{
			name: "default filters unspecified loopback linklocal and dedups",
			opts: LocalAddrOptions{},
			want: parseAddrs(t, "10.0.0.1", "2001:db8::1"),
		},
		{
			name: "include loopback and linklocal",
			opts: LocalAddrOptions{
				IncludeLoopback:  true,
				IncludeLinkLocal: true,
			},
			want: parseAddrs(t, "10.0.0.1", "::1", "2001:db8::1", "fe80::1"),
		},
		{
			name: "family ipv4 only",
			opts: LocalAddrOptions{Family: FamilyIPv4},
			want: parseAddrs(t, "10.0.0.1"),
		},
		{
			name: "family ipv6 only",
			opts: LocalAddrOptions{
				Family:           FamilyIPv6,
				IncludeLoopback:  true,
				IncludeLinkLocal: true,
			},
			want: parseAddrs(t, "::1", "2001:db8::1", "fe80::1"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := filterAndSortAddrs(raw, tc.opts)
			assertAddrSliceEqual(t, got, tc.want)
		})
	}
}

func TestSelectFromCandidates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		candidates []netip.Addr
		opts       LocalAddrOptions
		want       netip.Addr
		wantErr    error
	}{
		{
			name:       "default prefers private",
			candidates: parseAddrs(t, "8.8.8.8", "10.0.0.2", "2001:4860:4860::8888"),
			opts:       LocalAddrOptions{},
			want:       netip.MustParseAddr("10.0.0.2"),
		},
		{
			name:       "prefer public chooses public global unicast first",
			candidates: parseAddrs(t, "10.0.0.2", "8.8.8.8"),
			opts:       LocalAddrOptions{PreferPublic: true},
			want:       netip.MustParseAddr("8.8.8.8"),
		},
		{
			name:       "cgnat treated as private by default",
			candidates: parseAddrs(t, "8.8.8.8", "100.64.1.2"),
			opts:       LocalAddrOptions{},
			want:       netip.MustParseAddr("100.64.1.2"),
		},
		{
			name:       "exclude cgnat picks public",
			candidates: parseAddrs(t, "8.8.8.8", "100.64.1.2"),
			opts:       LocalAddrOptions{ExcludeCGNAT: true},
			want:       netip.MustParseAddr("8.8.8.8"),
		},
		{
			name:       "no candidates",
			candidates: nil,
			opts:       LocalAddrOptions{},
			wantErr:    ErrNoUsableAddress,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := selectFromCandidates(tc.candidates, tc.opts)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("selectFromCandidates() error = %v, want %v", err, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("selectFromCandidates() unexpected error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("selectFromCandidates() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestListLocalAddrs_StableNoDuplicatesNoUnspecified(t *testing.T) {
	t.Parallel()

	got, err := ListLocalAddrs(
		context.Background(),
		LocalAddrOptions{
			IncludeLoopback:  true,
			IncludeLinkLocal: true,
		},
	)
	if err != nil {
		t.Fatalf("ListLocalAddrs() unexpected error = %v", err)
	}

	seen := make(map[netip.Addr]struct{}, len(got))
	for i, addr := range got {
		if addr.IsUnspecified() {
			t.Fatalf("ListLocalAddrs() returned unspecified address: %s", addr)
		}
		if _, ok := seen[addr]; ok {
			t.Fatalf("ListLocalAddrs() returned duplicate address: %s", addr)
		}
		seen[addr] = struct{}{}

		if i == 0 {
			continue
		}
		if got[i-1].Compare(addr) > 0 {
			t.Fatalf("ListLocalAddrs() not sorted: %s > %s", got[i-1], addr)
		}
	}
}

func TestListLocalAddrs_CanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListLocalAddrs(ctx, LocalAddrOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListLocalAddrs() error = %v, want context.Canceled", err)
	}
}

func TestSelectLocalAddr_CanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := SelectLocalAddr(ctx, LocalAddrOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SelectLocalAddr() error = %v, want context.Canceled", err)
	}
}

func TestListenTCPRandom(t *testing.T) {
	t.Parallel()

	t.Run("tcp4 success", func(t *testing.T) {
		t.Parallel()

		listener, addrPort, err := ListenTCPRandom(
			context.Background(),
			ListenOptions{Network: "tcp4"},
		)
		if err != nil {
			t.Fatalf("ListenTCPRandom() unexpected error = %v", err)
		}
		defer func() {
			_ = listener.Close()
		}()

		if addrPort.Port() == 0 {
			t.Fatalf("ListenTCPRandom() returned zero port: %s", addrPort)
		}
		if !addrPort.Addr().Is4() {
			t.Fatalf("ListenTCPRandom() expected IPv4 address, got %s", addrPort.Addr())
		}
	})

	t.Run("invalid network", func(t *testing.T) {
		t.Parallel()

		_, _, err := ListenTCPRandom(context.Background(), ListenOptions{Network: "udp"})
		if err == nil {
			t.Fatal("ListenTCPRandom() expected error for invalid network")
		}
		if !strings.Contains(err.Error(), "invalid network") {
			t.Fatalf("ListenTCPRandom() error = %v, want invalid network", err)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := ListenTCPRandom(ctx, ListenOptions{Network: "tcp4"})
		if err == nil {
			t.Fatal("ListenTCPRandom() expected context cancellation error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ListenTCPRandom() error = %v, want context.Canceled", err)
		}
	})

	t.Run("tcp6 success or skip", func(t *testing.T) {
		t.Parallel()

		listener, addrPort, err := ListenTCPRandom(
			context.Background(),
			ListenOptions{Network: "tcp6"},
		)
		if err != nil {
			if isIPv6Unsupported(err) {
				t.Skipf("tcp6 not supported in current environment: %v", err)
			}
			t.Fatalf("ListenTCPRandom() unexpected error for tcp6: %v", err)
		}
		defer func() {
			_ = listener.Close()
		}()

		if addrPort.Port() == 0 {
			t.Fatalf("ListenTCPRandom() returned zero port for tcp6: %s", addrPort)
		}
		if !addrPort.Addr().Is6() {
			t.Fatalf("ListenTCPRandom() expected IPv6 address, got %s", addrPort.Addr())
		}
	})
}

func parseAddrs(t *testing.T, values ...string) []netip.Addr {
	t.Helper()

	addrs := make([]netip.Addr, 0, len(values))
	for _, value := range values {
		addr, err := netip.ParseAddr(value)
		if err != nil {
			t.Fatalf("failed to parse address %q: %v", value, err)
		}
		addrs = append(addrs, addr)
	}
	return addrs
}

func assertAddrSliceEqual(t *testing.T, got, want []netip.Addr) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("addr length = %d, want %d\n got: %v\nwant: %v", len(got), len(want), got, want)
	}

	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("addr[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func isIPv6Unsupported(err error) bool {
	msg := strings.ToLower(err.Error())
	unsupportedHints := []string{
		"address family not supported",
		"protocol not available",
		"cannot assign requested address",
		"no suitable address",
		"network is unreachable",
	}
	for _, hint := range unsupportedHints {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}
