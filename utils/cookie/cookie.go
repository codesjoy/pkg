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

// Package cookie provides helpers for parsing and formatting HTTP Cookie headers.
package cookie

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrInvalidCookieHeader indicates malformed cookie fragments in a Cookie header.
var ErrInvalidCookieHeader = errors.New("invalid cookie header")

// GetCookie extracts a cookie by name from a raw Cookie header string.
//
// It returns nil, nil when the cookie name does not exist.
// It returns an error when the header contains malformed non-empty cookie fragments.
func GetCookie(rawCookies, name string) (*http.Cookie, error) {
	if name == "" {
		return nil, nil
	}

	for i := 0; i < len(rawCookies); {
		part, next := nextCookiePart(rawCookies, i)
		i = next
		if part == "" {
			continue
		}

		cookieName, cookieValue, ok := parseCookiePart(part)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidCookieHeader, part)
		}
		if cookieName != name {
			continue
		}

		// Request cookies do not carry response-only security attributes.
		return &http.Cookie{ //nolint:gosec // parsed request cookie
			Name:  cookieName,
			Value: cookieValue,
		}, nil
	}

	return nil, nil
}

// Parse converts a raw Cookie header into []*http.Cookie.
//
// Invalid fragments are skipped because this API does not expose parse errors.
func Parse(rawCookies string) []*http.Cookie {
	if rawCookies == "" {
		return []*http.Cookie{}
	}

	cookies := make([]*http.Cookie, 0, strings.Count(rawCookies, ";")+1)
	for i := 0; i < len(rawCookies); {
		part, next := nextCookiePart(rawCookies, i)
		i = next
		if part == "" {
			continue
		}

		name, value, ok := parseCookiePart(part)
		if !ok {
			continue
		}

		// Request cookies do not carry response-only security attributes.
		cookies = append(
			cookies,
			&http.Cookie{Name: name, Value: value}, //nolint:gosec // parsed request cookie
		)
	}

	if len(cookies) == 0 {
		return []*http.Cookie{}
	}
	return cookies
}

// Format converts cookies to their String form.
func Format(cookies []*http.Cookie) []string {
	if len(cookies) == 0 {
		return []string{}
	}

	rawCookies := make([]string, 0, len(cookies))
	for _, item := range cookies {
		if item == nil {
			continue
		}
		rawCookies = append(rawCookies, item.String())
	}

	if len(rawCookies) == 0 {
		return []string{}
	}
	return rawCookies
}

func nextCookiePart(raw string, start int) (string, int) {
	n := len(raw)
	for start < n {
		c := raw[start]
		if c != ';' && c != ' ' && c != '\t' {
			break
		}
		start++
	}
	if start >= n {
		return "", n
	}

	end := start
	for end < n && raw[end] != ';' {
		end++
	}

	next := end
	if next < n && raw[next] == ';' {
		next++
	}

	return trimCookieWhitespace(raw[start:end]), next
}

func parseCookiePart(part string) (string, string, bool) {
	eq := strings.IndexByte(part, '=')
	if eq <= 0 {
		return "", "", false
	}

	name := trimCookieWhitespace(part[:eq])
	if name == "" {
		return "", "", false
	}

	value := trimCookieWhitespace(part[eq+1:])
	return name, value, true
}

func trimCookieWhitespace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}

	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}

	return s[start:end]
}
