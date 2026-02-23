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

package xemail

import "testing"

func TestIsValidEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{name: "simple", email: "user@example.com", want: true},
		{name: "plus tag", email: "user+tag@example.com", want: true},
		{name: "subdomain", email: "user@sub.mail.example.com", want: true},
		{name: "hyphen domain", email: "user@ex-ample.com", want: true},
		{name: "country tld", email: "user@example.co.uk", want: true},
		{name: "empty", email: "", want: false},
		{name: "missing at", email: "userexample.com", want: false},
		{name: "leading dot local", email: ".user@example.com", want: false},
		{name: "trailing dot local", email: "user.@example.com", want: false},
		{name: "double dot local", email: "user..name@example.com", want: false},
		{name: "label starts hyphen", email: "user@-example.com", want: false},
		{name: "label ends hyphen", email: "user@example-.com", want: false},
		{name: "numeric tld", email: "user@example.123", want: false},
		{name: "single char tld", email: "user@example.c", want: false},
		{name: "unicode", email: "user@exämple.com", want: false},
		{name: "space", email: "user @example.com", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := IsValidEmail(tc.email)
			if got != tc.want {
				t.Fatalf("IsValidEmail(%q) = %v, want %v", tc.email, got, tc.want)
			}
		})
	}
}

func BenchmarkIsValidEmail_Valid(b *testing.B) {
	email := "user.name+tag@example.com"
	for i := 0; i < b.N; i++ {
		_ = IsValidEmail(email)
	}
}

func BenchmarkIsValidEmail_Invalid(b *testing.B) {
	email := "user..name@example.com"
	for i := 0; i < b.N; i++ {
		_ = IsValidEmail(email)
	}
}
