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

package xcrypto

import (
	"crypto/hmac"
	"encoding/hex"
	"testing"
)

func TestHMACSHA256_KnownVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  []byte
		data []byte
		want string
	}{
		{
			name: "RFC4231 case 1",
			key:  bytesRepeat(0x0b, 20),
			data: []byte("Hi There"),
			want: "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7",
		},
		{
			name: "RFC4231 case 2",
			key:  []byte("Jefe"),
			data: []byte("what do ya want for nothing?"),
			want: "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843",
		},
		{
			name: "RFC4231 case 3",
			key:  bytesRepeat(0xaa, 20),
			data: bytesRepeat(0xdd, 50),
			want: "773ea91e36800e46854db8ebd09181a72959098b3ef8c122d9635514ced565fe",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := HMACSHA256(tc.key, tc.data)
			gotHex := hex.EncodeToString(got)
			if gotHex != tc.want {
				t.Fatalf("HMACSHA256() = %s, want %s", gotHex, tc.want)
			}
		})
	}
}

func TestHMACSHA256_Deterministic(t *testing.T) {
	t.Parallel()

	key := []byte("secret-key")
	data := []byte("message")

	got1 := HMACSHA256(key, data)
	got2 := HMACSHA256(key, data)

	if !hmac.Equal(got1, got2) {
		t.Fatal("HMACSHA256() should be deterministic")
	}
}

func TestHMACSHA256_DifferentInputs(t *testing.T) {
	t.Parallel()

	key := []byte("secret-key")
	data1 := []byte("message-1")
	data2 := []byte("message-2")

	got1 := HMACSHA256(key, data1)
	got2 := HMACSHA256(key, data2)

	if hmac.Equal(got1, got2) {
		t.Fatal("different messages must produce different MAC values")
	}
}

func BenchmarkHMACSHA256_Small(b *testing.B) {
	key := []byte("32-byte-secret-key-1234567890abcd")
	data := []byte("message")
	for i := 0; i < b.N; i++ {
		_ = HMACSHA256(key, data)
	}
}

func BenchmarkHMACSHA256_Medium(b *testing.B) {
	key := []byte("32-byte-secret-key-1234567890abcd")
	data := make([]byte, 1024)
	for i := 0; i < b.N; i++ {
		_ = HMACSHA256(key, data)
	}
}

func BenchmarkHMACSHA256_Large(b *testing.B) {
	key := []byte("32-byte-secret-key-1234567890abcd")
	data := make([]byte, 102400)
	for i := 0; i < b.N; i++ {
		_ = HMACSHA256(key, data)
	}
}

func bytesRepeat(v byte, n int) []byte {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = v
	}
	return buf
}
