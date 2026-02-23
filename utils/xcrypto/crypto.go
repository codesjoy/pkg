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

// Package xcrypto provides cryptographic functions for HMAC-SHA256 operations.
//
// This package offers a simplified interface for computing HMAC-SHA256 hashes,
// which are commonly used for:
//   - Message authentication
//   - API signature verification
//   - Token generation and validation
//   - Data integrity verification
//
// Security Best Practices:
//   - Always use cryptographically secure random key generators (crypto/rand)
//   - Key length should be at least 32 bytes (256 bits) for HMAC-SHA256
//   - Never hardcode keys in source code
//   - Use timing-safe comparison when verifying HMAC values (use hmac.Equal)
//   - Rotate keys periodically in production systems
package xcrypto

import (
	"crypto/hmac"
	"crypto/sha256"
)

// HMACSHA256 computes a HMAC-SHA256 hash of the data using the provided key.
//
// HMAC (Hash-based Message Authentication Code) provides both data integrity
// and authentication using a shared secret key. The SHA256 hash function
// produces a 32-byte (256-bit) output.
//
// Parameters:
//   - key: The secret key for HMAC. Should be at least 32 bytes for security.
//   - data: The message data to authenticate. Can be empty.
//
// Returns:
//   - []byte: The 32-byte HMAC-SHA256 digest.
//
// The returned hash can be used for:
//   - Verifying data integrity
//   - Authenticating API requests
//   - Generating secure tokens
//
// Example:
//
//	key := []byte("32-byte-secret-key-1234567890abcd")
//	data := []byte("message to authenticate")
//	mac := HMACSHA256(key, data)
//
//	// To verify later (timing-safe comparison):
//	expectedMAC := HMACSHA256(key, data)
//	if hmac.Equal(mac, expectedMAC) {
//	    // Valid
//	}
//
// Note: For timing-safe verification, always use hmac.Equal() to compare
// HMAC values to prevent timing attacks.
func HMACSHA256(key []byte, data []byte) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write(data)
	return hash.Sum(nil)
}
