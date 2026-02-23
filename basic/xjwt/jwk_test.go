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

package xjwt

import (
	"crypto/rsa"
	"encoding/json"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/stretchr/testify/assert"
)

// TestGenerateRsaKey tests RSA key generation
func TestGenerateRsaKey(t *testing.T) {
	t.Run("generate RSA key", func(t *testing.T) {
		key, err := GenerateRsaKey()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, key, "key should not be nil")
		assert.Equal(t, 2048, key.N.BitLen(), "key should be 2048 bits")
	})
}

// TestGenerateRsaJwk tests RSA JWK generation
func TestGenerateRsaJwk(t *testing.T) {
	t.Run("generate RSA JWK", func(t *testing.T) {
		jwkKey, err := GenerateRsaJwk()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, jwkKey, "JWK should not be nil")

		// Verify it's an RSA key
		rsaKey := new(rsa.PrivateKey)
		err = jwkKey.Raw(rsaKey)
		assert.NoError(t, err, "should extract raw RSA key")
		assert.NotNil(t, rsaKey, "RSA key should not be nil")
		assert.Equal(t, 2048, rsaKey.N.BitLen(), "RSA key should be 2048 bits")
	})
}

// TestGenerateRsaPublicJwk tests RSA public JWK generation
func TestGenerateRsaPublicJwk(t *testing.T) {
	t.Run("generate RSA public JWK", func(t *testing.T) {
		pubKey, err := GenerateRsaPublicJwk()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, pubKey, "public JWK should not be nil")

		// Verify it's a public key
		keyType := pubKey.KeyType()
		assert.Equal(t, jwa.RSA, keyType, "should be RSA key type")
	})
}

// TestECDSAKeyGeneration tests ECDSA key generation for all supported curves
func TestECDSAKeyGeneration(t *testing.T) {
	curves := []jwa.EllipticCurveAlgorithm{jwa.P256, jwa.P384, jwa.P521}

	for _, curve := range curves {
		t.Run(curve.String(), func(t *testing.T) {
			t.Run("generate ECDSA key", func(t *testing.T) {
				key, err := GenerateEcdsaKey(curve)
				assert.NoError(t, err, "should not return error")
				assert.NotNil(t, key, "key should not be nil")
				assert.NotNil(t, key.PublicKey, "public key should not be nil")
			})

			t.Run("generate ECDSA JWK", func(t *testing.T) {
				jwkKey, err := GenerateEcdsaJwk(curve)
				assert.NoError(t, err, "should not return error")
				assert.NotNil(t, jwkKey, "JWK should not be nil")

				// Verify curve in JWK - use type assertion to access Crv method
				if ecdsaKey, ok := jwkKey.(interface {
					Crv() jwa.EllipticCurveAlgorithm
				}); ok {
					crv := ecdsaKey.Crv()
					assert.Equal(t, curve, crv, "curve should match")
				} else {
					t.Fatalf("expected ECDSA key type")
				}
			})

			t.Run("generate ECDSA public JWK", func(t *testing.T) {
				pubKey, err := GenerateEcdsaPublicJwk(curve)
				assert.NoError(t, err, "should not return error")
				assert.NotNil(t, pubKey, "public JWK should not be nil")

				// Verify it's a public key
				keyType := pubKey.KeyType()
				assert.Equal(t, jwa.EC, keyType, "should be EC key type")
			})
		})
	}
}

// TestGenerateEcdsaJwkConvenience tests convenience functions for ECDSA JWK generation
func TestGenerateEcdsaJwkConvenience(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() (jwk.Key, error)
		expected jwa.EllipticCurveAlgorithm
	}{
		{"P256", GenerateEcdsaJwkP256, jwa.P256},
		{"P384", GenerateEcdsaJwkP384, jwa.P384},
		{"P521", GenerateEcdsaJwkP521, jwa.P521},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwkKey, err := tt.fn()
			assert.NoError(t, err, "should not return error")
			assert.NotNil(t, jwkKey, "JWK should not be nil")

			// Verify curve in JWK - use type assertion to access Crv method
			if ecdsaKey, ok := jwkKey.(interface {
				Crv() jwa.EllipticCurveAlgorithm
			}); ok {
				crv := ecdsaKey.Crv()
				assert.Equal(t, tt.expected, crv, "curve should match")
			} else {
				t.Fatalf("expected ECDSA key type")
			}
		})
	}
}

// TestGenerateEd25519Key tests Ed25519 key generation
func TestGenerateEd25519Key(t *testing.T) {
	t.Run("generate Ed25519 key", func(t *testing.T) {
		key, err := GenerateEd25519Key()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, key, "key should not be nil")
		assert.Equal(t, 64, len(key), "Ed25519 private key should be 64 bytes (seed + public key)")
	})

	t.Run("generate Ed25519 JWK", func(t *testing.T) {
		jwkKey, err := GenerateEd25519Jwk()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, jwkKey, "JWK should not be nil")

		// Verify it's an OKP (Octet Key Pair) key
		keyType := jwkKey.KeyType()
		assert.Equal(t, jwa.OKP, keyType, "should be OKP key type")
	})
}

// TestGenerateX25519Key tests X25519 key generation
func TestGenerateX25519Key(t *testing.T) {
	t.Run("generate X25519 key", func(t *testing.T) {
		key, err := GenerateX25519Key()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, key, "key should not be nil")
		assert.Equal(t, 64, len(key), "X25519 private key should be 64 bytes (seed + public key)")
	})

	t.Run("generate X25519 JWK", func(t *testing.T) {
		jwkKey, err := GenerateX25519Jwk()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, jwkKey, "JWK should not be nil")

		// Verify it's an OKP (Octet Key Pair) key
		keyType := jwkKey.KeyType()
		assert.Equal(t, jwa.OKP, keyType, "should be OKP key type")
	})
}

// TestGenerateSymmetricKey tests symmetric key generation
func TestGenerateSymmetricKey(t *testing.T) {
	t.Run("generate symmetric key", func(t *testing.T) {
		key, err := GenerateSymmetricKey()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, key, "key should not be nil")
		assert.Equal(t, 64, len(key), "symmetric key should be 64 bytes")
	})

	t.Run("generate symmetric JWK", func(t *testing.T) {
		jwkKey, err := GenerateSymmetricJwk()
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, jwkKey, "JWK should not be nil")

		// Verify it's a symmetric key
		keyType := jwkKey.KeyType()
		assert.Equal(t, jwa.OctetSeq, keyType, "should be octet sequence key type")
	})
}

// TestJWKSerialization tests JWK serialization and deserialization
func TestJWKSerialization(t *testing.T) {
	t.Run("RSA JWK serialization", func(t *testing.T) {
		jwkKey, err := GenerateRsaJwk()
		assert.NoError(t, err)

		// Serialize to JSON
		jsonBytes, err := json.Marshal(jwkKey)
		assert.NoError(t, err, "should serialize to JSON")
		assert.NotEmpty(t, jsonBytes, "JSON should not be empty")

		// Deserialize from JSON
		parsedKey, err := jwk.ParseKey(jsonBytes)
		assert.NoError(t, err, "should deserialize from JSON")
		assert.NotNil(t, parsedKey, "parsed key should not be nil")
	})

	t.Run("ECDSA JWK serialization", func(t *testing.T) {
		jwkKey, err := GenerateEcdsaJwk(jwa.P256)
		assert.NoError(t, err)

		// Serialize to JSON
		jsonBytes, err := json.Marshal(jwkKey)
		assert.NoError(t, err, "should serialize to JSON")
		assert.NotEmpty(t, jsonBytes, "JSON should not be empty")

		// Deserialize from JSON
		parsedKey, err := jwk.ParseKey(jsonBytes)
		assert.NoError(t, err, "should deserialize from JSON")
		assert.NotNil(t, parsedKey, "parsed key should not be nil")
	})

	t.Run("Ed25519 JWK serialization", func(t *testing.T) {
		jwkKey, err := GenerateEd25519Jwk()
		assert.NoError(t, err)

		// Serialize to JSON
		jsonBytes, err := json.Marshal(jwkKey)
		assert.NoError(t, err, "should serialize to JSON")
		assert.NotEmpty(t, jsonBytes, "JSON should not be empty")

		// Deserialize from JSON
		parsedKey, err := jwk.ParseKey(jsonBytes)
		assert.NoError(t, err, "should deserialize from JSON")
		assert.NotNil(t, parsedKey, "parsed key should not be nil")
	})
}

// TestPublicKeyExtraction tests public key extraction from private keys
func TestPublicKeyExtraction(t *testing.T) {
	t.Run("RSA public key extraction", func(t *testing.T) {
		privKey, err := GenerateRsaJwk()
		assert.NoError(t, err)

		pubKey, err := jwk.PublicKeyOf(privKey)
		assert.NoError(t, err, "should extract public key")
		assert.NotNil(t, pubKey, "public key should not be nil")

		// Verify it's a public key (no private parameters)
		_, hasD := pubKey.Get("d")
		assert.False(t, hasD, "public key should not have private parameter 'd'")
	})

	t.Run("ECDSA public key extraction", func(t *testing.T) {
		privKey, err := GenerateEcdsaJwk(jwa.P384)
		assert.NoError(t, err)

		pubKey, err := jwk.PublicKeyOf(privKey)
		assert.NoError(t, err, "should extract public key")
		assert.NotNil(t, pubKey, "public key should not be nil")

		// Verify it's a public key (no private parameters)
		_, hasD := pubKey.Get("d")
		assert.False(t, hasD, "public key should not have private parameter 'd'")
	})
}

// TestConcurrentKeyGeneration tests concurrent key generation
func TestConcurrentKeyGeneration(t *testing.T) {
	const numGoroutines = 50

	t.Run("concurrent RSA key generation", func(t *testing.T) {
		var counter atomic.Int32
		var errCount atomic.Int32

		for i := 0; i < numGoroutines; i++ {
			go func() {
				_, err := GenerateRsaJwk()
				counter.Add(1)
				if err != nil {
					errCount.Add(1)
				}
			}()
		}

		// Wait for all goroutines to complete
		for counter.Load() < numGoroutines {
			runtime.Gosched()
		}

		assert.Equal(t, int32(0), errCount.Load(), "no errors should occur")
		assert.Equal(t, int32(numGoroutines), counter.Load(), "all goroutines should complete")
	})

	t.Run("concurrent ECDSA key generation", func(t *testing.T) {
		var counter atomic.Int32
		var errCount atomic.Int32

		for i := 0; i < numGoroutines; i++ {
			go func() {
				_, err := GenerateEcdsaJwk(jwa.P256)
				counter.Add(1)
				if err != nil {
					errCount.Add(1)
				}
			}()
		}

		// Wait for all goroutines to complete
		for counter.Load() < numGoroutines {
			runtime.Gosched()
		}

		assert.Equal(t, int32(0), errCount.Load(), "no errors should occur")
		assert.Equal(t, int32(numGoroutines), counter.Load(), "all goroutines should complete")
	})
}

// TestInvalidCurve tests error handling for invalid curve
func TestInvalidCurve(t *testing.T) {
	t.Run("invalid ECDSA curve", func(t *testing.T) {
		// Try to use Ed25519 (which is not an ECDSA curve)
		_, err := GenerateEcdsaKey(jwa.Ed25519)
		assert.Error(t, err, "should return error for invalid curve")
		assert.Contains(
			t,
			err.Error(),
			"invalid curve algorithm",
			"error should mention invalid curve",
		)
	})
}
