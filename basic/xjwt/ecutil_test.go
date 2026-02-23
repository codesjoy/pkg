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
	"crypto/ecdsa"
	"crypto/elliptic"
	"math/big"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/stretchr/testify/assert"
)

// TestInitRegistersCurves tests that init() properly registers all standard curves
func TestInitRegistersCurves(t *testing.T) {
	t.Run("P256 curve is registered", func(t *testing.T) {
		crv, ok := CurveForAlgorithm(jwa.P256)
		assert.True(t, ok, "P256 should be registered")
		assert.NotNil(t, crv, "P256 curve should not be nil")
		assert.Equal(t, elliptic.P256(), crv, "P256 curve should match elliptic.P256()")
	})

	t.Run("P384 curve is registered", func(t *testing.T) {
		crv, ok := CurveForAlgorithm(jwa.P384)
		assert.True(t, ok, "P384 should be registered")
		assert.NotNil(t, crv, "P384 curve should not be nil")
		assert.Equal(t, elliptic.P384(), crv, "P384 curve should match elliptic.P384()")
	})

	t.Run("P521 curve is registered", func(t *testing.T) {
		crv, ok := CurveForAlgorithm(jwa.P521)
		assert.True(t, ok, "P521 should be registered")
		assert.NotNil(t, crv, "P521 curve should not be nil")
		assert.Equal(t, elliptic.P521(), crv, "P521 curve should match elliptic.P521()")
	})
}

// TestAvailableAlgorithms tests that all expected algorithms are available
func TestAvailableAlgorithms(t *testing.T) {
	algs := AvailableAlgorithms()
	assert.Contains(t, algs, jwa.P256, "P256 should be in available algorithms")
	assert.Contains(t, algs, jwa.P384, "P384 should be in available algorithms")
	assert.Contains(t, algs, jwa.P521, "P521 should be in available algorithms")
}

// TestAvailableCurves tests that all expected curves are available
func TestAvailableCurves(t *testing.T) {
	curves := AvailableCurves()
	assert.Contains(t, curves, elliptic.P256(), "P256 should be in available curves")
	assert.Contains(t, curves, elliptic.P384(), "P384 should be in available curves")
	assert.Contains(t, curves, elliptic.P521(), "P521 should be in available curves")
}

// TestIsAvailable tests the IsAvailable function
func TestIsAvailable(t *testing.T) {
	t.Run("available algorithms return true", func(t *testing.T) {
		assert.True(t, IsAvailable(jwa.P256), "P256 should be available")
		assert.True(t, IsAvailable(jwa.P384), "P384 should be available")
		assert.True(t, IsAvailable(jwa.P521), "P521 should be available")
	})

	t.Run("unavailable algorithms return false", func(t *testing.T) {
		// Ed25519 and X25519 are not ECDSA curves, so they shouldn't be registered
		assert.False(t, IsAvailable(jwa.Ed25519), "Ed25519 should not be available for ECDSA")
		assert.False(t, IsAvailable(jwa.X25519), "X25519 should not be available for ECDSA")
	})
}

// TestCurveForAlgorithm tests bidirectional curve-algorithm mapping
func TestCurveForAlgorithm(t *testing.T) {
	tests := []struct {
		name        string
		alg         jwa.EllipticCurveAlgorithm
		expectedCrv elliptic.Curve
		shouldExist bool
	}{
		{"P256", jwa.P256, elliptic.P256(), true},
		{"P384", jwa.P384, elliptic.P384(), true},
		{"P521", jwa.P521, elliptic.P521(), true},
		{"Ed25519 (not ECDSA)", jwa.Ed25519, nil, false},
		{"X25519 (not ECDSA)", jwa.X25519, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crv, ok := CurveForAlgorithm(tt.alg)
			if tt.shouldExist {
				assert.True(t, ok, "algorithm should be found")
				assert.Equal(t, tt.expectedCrv, crv, "curve should match")
			} else {
				assert.False(t, ok, "algorithm should not be found")
			}
		})
	}
}

// TestAlgorithmForCurve tests bidirectional curve-algorithm mapping
func TestAlgorithmForCurve(t *testing.T) {
	tests := []struct {
		name        string
		crv         elliptic.Curve
		expectedAlg jwa.EllipticCurveAlgorithm
		shouldExist bool
	}{
		{"P256", elliptic.P256(), jwa.P256, true},
		{"P384", elliptic.P384(), jwa.P384, true},
		{"P521", elliptic.P521(), jwa.P521, true},
		{"P224 (not registered)", elliptic.P224(), "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alg, ok := AlgorithmForCurve(tt.crv)
			if tt.shouldExist {
				assert.True(t, ok, "curve should be found")
				assert.Equal(t, tt.expectedAlg, alg, "algorithm should match")
			} else {
				assert.False(t, ok, "curve should not be found")
			}
		})
	}
}

// TestRegisterCurve tests custom curve registration
func TestRegisterCurve(t *testing.T) {
	// Create a custom curve (using P224 as an example, though it's not in JWT standard)
	customCurve := elliptic.P224()
	var customAlg jwa.EllipticCurveAlgorithm = "P-224"

	t.Run("register custom curve", func(t *testing.T) {
		// Before registration
		_, ok := CurveForAlgorithm(customAlg)
		assert.False(t, ok, "custom algorithm should not be registered initially")

		// Register
		RegisterCurve(customCurve, customAlg)

		// After registration
		crv, ok := CurveForAlgorithm(customAlg)
		assert.True(t, ok, "custom algorithm should be registered")
		assert.Equal(t, customCurve, crv, "custom curve should match")

		alg, ok := AlgorithmForCurve(customCurve)
		assert.True(t, ok, "custom curve should map to algorithm")
		assert.Equal(t, customAlg, alg, "custom algorithm should match")
	})
}

// TestBufferPool tests the EC point buffer pool
func TestBufferPool(t *testing.T) {
	t.Run("allocate and release buffer", func(t *testing.T) {
		// Create a big integer for testing
		value := new(big.Int).SetBytes([]byte{1, 2, 3, 4, 5})

		// Allocate buffer
		buf := AllocECPointBuffer(value, elliptic.P256())
		assert.NotNil(t, buf, "buffer should not be nil")
		assert.NotEmpty(t, buf, "buffer should not be empty")

		// Release buffer
		ReleaseECPointBuffer(buf)
		// If we reach here without panic, the release worked
	})

	t.Run("concurrent buffer allocation", func(t *testing.T) {
		const numGoroutines = 100
		var counter atomic.Int32

		for i := 0; i < numGoroutines; i++ {
			go func() {
				value := new(big.Int).SetBytes([]byte{1, 2, 3, 4, 5})
				buf := AllocECPointBuffer(value, elliptic.P256())
				if buf != nil {
					counter.Add(1)
				}
				ReleaseECPointBuffer(buf)
			}()
		}

		// Wait for all goroutines to complete
		for counter.Load() < numGoroutines {
			runtime.Gosched()
		}

		assert.Equal(t, int32(numGoroutines), counter.Load(), "all goroutines should complete")
	})

	t.Run("buffer size for different curves", func(t *testing.T) {
		value := new(big.Int).SetBytes([]byte{
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		})

		// Test P256 (256 bits = 32 bytes)
		buf256 := AllocECPointBuffer(value, elliptic.P256())
		assert.Equal(t, 32, len(buf256), "P256 buffer should be 32 bytes")
		ReleaseECPointBuffer(buf256)

		// Test P384 (384 bits = 48 bytes)
		buf384 := AllocECPointBuffer(value, elliptic.P384())
		assert.Equal(t, 48, len(buf384), "P384 buffer should be 48 bytes")
		ReleaseECPointBuffer(buf384)

		// Test P521 (521 bits needs special handling = 66 bytes)
		buf521 := AllocECPointBuffer(value, elliptic.P521())
		assert.Equal(t, 66, len(buf521), "P521 buffer should be 66 bytes")
		ReleaseECPointBuffer(buf521)
	})
}

// TestGenerateEcdsaKeyUsesRegisteredCurves tests that key generation uses registered curves
func TestGenerateEcdsaKeyUsesRegisteredCurves(t *testing.T) {
	t.Run("generate key with registered curve", func(t *testing.T) {
		key, err := GenerateEcdsaKey(jwa.P256)
		assert.NoError(t, err, "should not return error")
		assert.NotNil(t, key, "key should not be nil")
		assert.IsType(t, &ecdsa.PrivateKey{}, key, "should be ECDSA private key")
		assert.NotNil(t, key.PublicKey, "public key should not be nil")
		assert.NotNil(t, key.Curve, "curve should not be nil")
		assert.Equal(t, elliptic.P256(), key.Curve, "curve should be P256")
	})
}
