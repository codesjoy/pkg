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

// Package xnet provides high-level network helpers on top of Go stdlib.
package xnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
)

// Family controls IP family filtering for local address selection.
type Family uint8

const (
	// FamilyAny allows both IPv4 and IPv6.
	FamilyAny Family = iota
	// FamilyIPv4 allows only IPv4 addresses.
	FamilyIPv4
	// FamilyIPv6 allows only IPv6 addresses.
	FamilyIPv6
)

// LocalAddrOptions controls local address listing and selection behavior.
type LocalAddrOptions struct {
	// Family filters address family. Zero value means FamilyAny.
	Family Family
	// PreferPublic prefers public global unicast addresses when true.
	PreferPublic bool
	// ExcludeCGNAT excludes 100.64.0.0/10 from private candidates when true.
	ExcludeCGNAT bool
	// IncludeLoopback includes loopback addresses when true.
	IncludeLoopback bool
	// IncludeLinkLocal includes link-local addresses when true.
	IncludeLinkLocal bool
}

// ListenOptions controls random TCP listen behavior.
type ListenOptions struct {
	// Network must be "", "tcp", "tcp4", or "tcp6".
	Network string
	// Host defaults to 127.0.0.1 for tcp/tcp4 and ::1 for tcp6.
	Host string
}

// ErrNoUsableAddress indicates no address matches requested selection policy.
var ErrNoUsableAddress = errors.New("xnet: no usable local address")

var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// ListLocalAddrs returns unique local addresses filtered and sorted by options.
func ListLocalAddrs(ctx context.Context, opts LocalAddrOptions) ([]netip.Addr, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("xnet: list interfaces: %w", err)
	}

	rawAddrs := make([]netip.Addr, 0, len(ifaces))
	for _, iface := range ifaces {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		ifaceAddrs, err := iface.Addrs()
		if err != nil {
			// Network interfaces can disappear at runtime; skip transient failures.
			continue
		}

		for _, ifaceAddr := range ifaceAddrs {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			ip, ok := addrToIP(ifaceAddr)
			if !ok {
				continue
			}

			addr, ok := netip.AddrFromSlice(ip)
			if !ok {
				continue
			}
			rawAddrs = append(rawAddrs, addr)
		}
	}

	return filterAndSortAddrs(rawAddrs, opts), nil
}

// SelectLocalAddr selects one local address according to LocalAddrOptions.
func SelectLocalAddr(ctx context.Context, opts LocalAddrOptions) (netip.Addr, error) {
	addrs, err := ListLocalAddrs(ctx, opts)
	if err != nil {
		return netip.Addr{}, err
	}
	return selectFromCandidates(addrs, opts)
}

// ListenTCPRandom listens on a random TCP port and returns an open listener.
func ListenTCPRandom(
	ctx context.Context,
	opts ListenOptions,
) (net.Listener, netip.AddrPort, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, netip.AddrPort{}, err
	}

	network, err := normalizeListenNetwork(opts.Network)
	if err != nil {
		return nil, netip.AddrPort{}, err
	}

	host := opts.Host
	if host == "" {
		host = defaultHostForNetwork(network)
	}

	listenAddr := net.JoinHostPort(host, "0")
	listener, err := (&net.ListenConfig{}).Listen(ctx, network, listenAddr)
	if err != nil {
		return nil, netip.AddrPort{}, fmt.Errorf("xnet: listen %s %q: %w", network, listenAddr, err)
	}

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, netip.AddrPort{}, fmt.Errorf(
			"xnet: unexpected listener address type: %T",
			listener.Addr(),
		)
	}

	return listener, tcpAddr.AddrPort(), nil
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizeListenNetwork(network string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(network))
	if normalized == "" {
		return "tcp", nil
	}

	switch normalized {
	case "tcp", "tcp4", "tcp6":
		return normalized, nil
	default:
		return "", fmt.Errorf("xnet: invalid network %q: must be tcp, tcp4, or tcp6", network)
	}
}

func defaultHostForNetwork(network string) string {
	if network == "tcp6" {
		return "::1"
	}
	return "127.0.0.1"
}

func addrToIP(addr net.Addr) (net.IP, bool) {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP, true
	case *net.IPAddr:
		return v.IP, true
	default:
		return nil, false
	}
}

func filterAndSortAddrs(rawAddrs []netip.Addr, opts LocalAddrOptions) []netip.Addr {
	if len(rawAddrs) == 0 {
		return []netip.Addr{}
	}

	seen := make(map[netip.Addr]struct{}, len(rawAddrs))
	filtered := make([]netip.Addr, 0, len(rawAddrs))

	for _, rawAddr := range rawAddrs {
		addr := rawAddr.Unmap()
		if !addr.IsValid() || addr.IsUnspecified() {
			continue
		}

		switch opts.Family {
		case FamilyIPv4:
			if !addr.Is4() {
				continue
			}
		case FamilyIPv6:
			if !addr.Is6() {
				continue
			}
		}

		if !opts.IncludeLoopback && addr.IsLoopback() {
			continue
		}
		if !opts.IncludeLinkLocal && (addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast()) {
			continue
		}

		if _, exists := seen[addr]; exists {
			continue
		}
		seen[addr] = struct{}{}
		filtered = append(filtered, addr)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Compare(filtered[j]) < 0
	})

	return filtered
}

func selectFromCandidates(candidates []netip.Addr, opts LocalAddrOptions) (netip.Addr, error) {
	if len(candidates) == 0 {
		return netip.Addr{}, ErrNoUsableAddress
	}

	privateCandidates := make([]netip.Addr, 0, len(candidates))
	publicCandidates := make([]netip.Addr, 0, len(candidates))
	for _, addr := range candidates {
		if isPrivateCandidate(addr, opts.ExcludeCGNAT) {
			privateCandidates = append(privateCandidates, addr)
			continue
		}
		if addr.IsGlobalUnicast() {
			publicCandidates = append(publicCandidates, addr)
		}
	}

	if opts.PreferPublic {
		if len(publicCandidates) > 0 {
			return publicCandidates[0], nil
		}
		if len(privateCandidates) > 0 {
			return privateCandidates[0], nil
		}
		return candidates[0], nil
	}

	if len(privateCandidates) > 0 {
		return privateCandidates[0], nil
	}
	if len(publicCandidates) > 0 {
		return publicCandidates[0], nil
	}
	return candidates[0], nil
}

func isPrivateCandidate(addr netip.Addr, excludeCGNAT bool) bool {
	addr = addr.Unmap()
	if addr.IsPrivate() {
		return true
	}
	if excludeCGNAT || !addr.Is4() {
		return false
	}
	return cgnatPrefix.Contains(addr)
}
