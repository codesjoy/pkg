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

package cookie

import (
	"errors"
	"net/http"
	"testing"
)

func TestGetCookie(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		cookieName string
		wantValue  string
		wantNil    bool
		wantErr    error
	}{
		{
			name:       "found",
			raw:        "session=abc123; user=john",
			cookieName: "session",
			wantValue:  "abc123",
		},
		{
			name:       "not found",
			raw:        "session=abc123; user=john",
			cookieName: "missing",
			wantNil:    true,
		},
		{
			name:       "empty raw",
			raw:        "",
			cookieName: "session",
			wantNil:    true,
		},
		{
			name:       "malformed part",
			raw:        "session=abc123; malformed; user=john",
			cookieName: "user",
			wantErr:    ErrInvalidCookieHeader,
		},
		{
			name:       "value contains equals",
			raw:        "data=a=b=c; user=john",
			cookieName: "data",
			wantValue:  "a=b=c",
		},
		{
			name:       "leading whitespace",
			raw:        " session=abc123 ; user=john ",
			cookieName: "user",
			wantValue:  "john",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := GetCookie(tc.raw, tc.cookieName)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("GetCookie() error = %v, want %v", err, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetCookie() unexpected error = %v", err)
			}

			if tc.wantNil {
				if got != nil {
					t.Fatalf("GetCookie() cookie = %#v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("GetCookie() returned nil cookie")
			}
			if got.Name != tc.cookieName {
				t.Fatalf("GetCookie() name = %q, want %q", got.Name, tc.cookieName)
			}
			if got.Value != tc.wantValue {
				t.Fatalf("GetCookie() value = %q, want %q", got.Value, tc.wantValue)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantNames []string
		wantVals  []string
	}{
		{
			name:      "single",
			raw:       "session=abc123",
			wantNames: []string{"session"},
			wantVals:  []string{"abc123"},
		},
		{
			name:      "multiple",
			raw:       "session=abc123; user=john; theme=dark",
			wantNames: []string{"session", "user", "theme"},
			wantVals:  []string{"abc123", "john", "dark"},
		},
		{
			name:      "skip malformed",
			raw:       "session=abc123; malformed; user=john; =bad; theme=dark",
			wantNames: []string{"session", "user", "theme"},
			wantVals:  []string{"abc123", "john", "dark"},
		},
		{
			name:      "empty",
			raw:       "",
			wantNames: []string{},
			wantVals:  []string{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Parse(tc.raw)
			if len(got) != len(tc.wantNames) {
				t.Fatalf("Parse() len = %d, want %d", len(got), len(tc.wantNames))
			}
			for i := range got {
				if got[i].Name != tc.wantNames[i] {
					t.Fatalf(
						"Parse() cookie[%d].Name = %q, want %q",
						i,
						got[i].Name,
						tc.wantNames[i],
					)
				}
				if got[i].Value != tc.wantVals[i] {
					t.Fatalf(
						"Parse() cookie[%d].Value = %q, want %q",
						i,
						got[i].Value,
						tc.wantVals[i],
					)
				}
			}
		})
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []*http.Cookie
		want  []string
	}{
		{
			name: "normal",
			input: []*http.Cookie{
				// Request cookies intentionally omit response-only security attributes.
				{Name: "session", Value: "abc123"}, //nolint:gosec // plain request cookie fixture
				{Name: "user", Value: "john"},      //nolint:gosec // plain request cookie fixture
			},
			want: []string{"session=abc123", "user=john"},
		},
		{
			name: "skip nil",
			input: []*http.Cookie{
				nil,
				// Request cookies intentionally omit response-only security attributes.
				{Name: "user", Value: "john"}, //nolint:gosec // plain request cookie fixture
			},
			want: []string{"user=john"},
		},
		{
			name:  "empty",
			input: []*http.Cookie{},
			want:  []string{},
		},
		{
			name:  "nil slice",
			input: nil,
			want:  []string{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Format(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("Format() len = %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("Format() item[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	raw := "session=abc123; user=john; theme=dark"
	cookies := Parse(raw)
	formatted := Format(cookies)
	if len(formatted) != 3 {
		t.Fatalf("round-trip format len = %d, want 3", len(formatted))
	}

	joined := formatted[0] + "; " + formatted[1] + "; " + formatted[2]
	got := Parse(joined)
	if len(got) != 3 {
		t.Fatalf("round-trip parse len = %d, want 3", len(got))
	}
}

func BenchmarkGetCookie_Hit(b *testing.B) {
	raw := "session=abc123; user=john; theme=dark; lang=en"
	for i := 0; i < b.N; i++ {
		_, _ = GetCookie(raw, "user")
	}
}

func BenchmarkGetCookie_Miss(b *testing.B) {
	raw := "session=abc123; user=john; theme=dark; lang=en"
	for i := 0; i < b.N; i++ {
		_, _ = GetCookie(raw, "unknown")
	}
}

func BenchmarkParse(b *testing.B) {
	raw := "session=abc123; user=john; theme=dark; lang=en"
	for i := 0; i < b.N; i++ {
		_ = Parse(raw)
	}
}

func BenchmarkFormat(b *testing.B) {
	cookies := []*http.Cookie{
		{Name: "session", Value: "abc123"}, //nolint:gosec // plain request cookie fixture
		{Name: "user", Value: "john"},      //nolint:gosec // plain request cookie fixture
		{Name: "theme", Value: "dark"},     //nolint:gosec // plain request cookie fixture
		{Name: "lang", Value: "en"},        //nolint:gosec // plain request cookie fixture
	}
	for i := 0; i < b.N; i++ {
		_ = Format(cookies)
	}
}
