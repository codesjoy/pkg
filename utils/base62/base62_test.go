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

package base62

import (
	"errors"
	"math"
	"strconv"
	"testing"
)

func TestEncodeInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   int64
		want    string
		wantErr error
	}{
		{name: "zero", input: 0, want: "a"},
		{name: "one", input: 1, want: "b"},
		{name: "sixty one", input: 61, want: "9"},
		{name: "sixty two", input: 62, want: "ba"},
		{name: "max int64", input: math.MaxInt64, want: "k9viXaIfiWh"},
		{name: "negative", input: -1, wantErr: ErrNegativeNumber},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EncodeInt64(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("EncodeInt64() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("EncodeInt64() unexpected error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("EncodeInt64() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr error
	}{
		{name: "zero", input: "a", want: 0},
		{name: "one", input: "b", want: 1},
		{name: "sixty one", input: "9", want: 61},
		{name: "sixty two", input: "ba", want: 62},
		{name: "max int64", input: "k9viXaIfiWh", want: math.MaxInt64},
		{name: "empty", input: "", wantErr: ErrEmptyString},
		{name: "invalid char", input: "abc!", wantErr: ErrInvalidCharacter},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeInt64(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("DecodeInt64() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeInt64() unexpected error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("DecodeInt64() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDecodeInt64_Overflow(t *testing.T) {
	_, err := DecodeInt64("zzzzzzzzzzzz")
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("DecodeInt64() error = %v, want overflow", err)
	}
}

func TestRoundTrip(t *testing.T) {
	values := []int64{0, 1, 10, 61, 62, 12345, 999999, 1 << 40, math.MaxInt64}

	for _, v := range values {
		t.Run(strconv.FormatInt(v, 10), func(t *testing.T) {
			encoded, err := EncodeInt64(v)
			if err != nil {
				t.Fatalf("EncodeInt64() error = %v", err)
			}

			decoded, err := DecodeInt64(encoded)
			if err != nil {
				t.Fatalf("DecodeInt64() error = %v", err)
			}

			if decoded != v {
				t.Fatalf("round trip = %d, want %d", decoded, v)
			}
		})
	}
}

func BenchmarkEncodeInt64(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = EncodeInt64(123456789)
	}
}

func BenchmarkDecodeInt64(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = DecodeInt64("iwaUH")
	}
}
