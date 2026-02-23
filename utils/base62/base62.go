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

// Package base62 provides standard base62 encoding and decoding for int64 values.
package base62

import (
	"errors"
	"fmt"
	"math"
)

const (
	base = int64(62)

	// Alphabet is the character set used by this package.
	Alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var (
	// ErrNegativeNumber is returned when trying to encode a negative number.
	ErrNegativeNumber = errors.New("base62: negative numbers are not supported")
	// ErrEmptyString is returned when trying to decode an empty string.
	ErrEmptyString = errors.New("base62: empty string")
	// ErrInvalidCharacter is returned when decoding a string with non-base62 characters.
	ErrInvalidCharacter = errors.New("base62: invalid character")
	// ErrOverflow is returned when decoded value exceeds int64 range.
	ErrOverflow = errors.New("base62: value overflows int64")
)

var decodeTable = initDecodeTable()

func initDecodeTable() [256]int16 {
	var table [256]int16
	for i := range table {
		table[i] = -1
	}
	for i := range Alphabet {
		table[Alphabet[i]] = int16(i)
	}
	return table
}

// EncodeInt64 converts n to a base62 string using standard most-significant-digit-first order.
func EncodeInt64(n int64) (string, error) {
	if n < 0 {
		return "", ErrNegativeNumber
	}
	if n == 0 {
		return string(Alphabet[0]), nil
	}

	var buf [11]byte
	idx := len(buf)
	for n > 0 {
		idx--
		buf[idx] = Alphabet[int(n%base)]
		n /= base
	}

	return string(buf[idx:]), nil
}

// DecodeInt64 converts a base62 string back to int64.
func DecodeInt64(s string) (int64, error) {
	if s == "" {
		return 0, ErrEmptyString
	}

	var result int64
	for i := range s {
		digit := decodeTable[s[i]]
		if digit < 0 {
			return 0, fmt.Errorf("%w: %q at index %d", ErrInvalidCharacter, s[i], i)
		}

		d := int64(digit)
		if result > (math.MaxInt64-d)/base {
			return 0, ErrOverflow
		}

		result = result*base + d
	}

	return result, nil
}
