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

// Package xjwt provides functions for generating JWT keys.
package xjwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/x25519"
)

// GenerateRsaKey generates a 2048-bit RSA private key.
func GenerateRsaKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// GenerateRsaJwk generates an RSA JWK (JSON Web Key).
func GenerateRsaJwk() (jwk.Key, error) {
	key, err := GenerateRsaKey()
	if err != nil {
		return nil, fmt.Errorf(`failed to generate RSA private key: %w`, err)
	}

	k, err := jwk.FromRaw(key)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate jwk.RSAPrivateKey: %w`, err)
	}

	return k, nil
}

// GenerateRsaPublicJwk generates a public JWK from an RSA private key.
func GenerateRsaPublicJwk() (jwk.Key, error) {
	key, err := GenerateRsaJwk()
	if err != nil {
		return nil, fmt.Errorf(`failed to generate jwk.RSAPrivateKey: %w`, err)
	}

	return jwk.PublicKeyOf(key)
}

// GenerateEcdsaKey generates an ECDSA private key for the specified elliptic curve algorithm.
func GenerateEcdsaKey(alg jwa.EllipticCurveAlgorithm) (*ecdsa.PrivateKey, error) {
	var crv elliptic.Curve
	if tmp, ok := CurveForAlgorithm(alg); ok {
		crv = tmp
	} else {
		return nil, fmt.Errorf(`invalid curve algorithm %s`, alg)
	}

	return ecdsa.GenerateKey(crv, rand.Reader)
}

// GenerateEcdsaJwk generates an ECDSA JWK for the specified curve (P256, P384, P521).
func GenerateEcdsaJwk(alg jwa.EllipticCurveAlgorithm) (jwk.Key, error) {
	key, err := GenerateEcdsaKey(alg)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate ECDSA private key: %w`, err)
	}

	k, err := jwk.FromRaw(key)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate jwk.ECDSAPrivateKey: %w`, err)
	}

	return k, nil
}

// GenerateEcdsaJwkP256 generates a P256 ECDSA JWK.
func GenerateEcdsaJwkP256() (jwk.Key, error) {
	return GenerateEcdsaJwk(jwa.P256)
}

// GenerateEcdsaJwkP384 generates a P384 ECDSA JWK.
func GenerateEcdsaJwkP384() (jwk.Key, error) {
	return GenerateEcdsaJwk(jwa.P384)
}

// GenerateEcdsaJwkP521 generates a P521 ECDSA JWK.
func GenerateEcdsaJwkP521() (jwk.Key, error) {
	return GenerateEcdsaJwk(jwa.P521)
}

// GenerateEcdsaPublicJwk generates a public JWK from an ECDSA private key.
func GenerateEcdsaPublicJwk(alg jwa.EllipticCurveAlgorithm) (jwk.Key, error) {
	key, err := GenerateEcdsaJwk(alg)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate jwk.ECDSAPrivateKey: %w`, err)
	}

	return jwk.PublicKeyOf(key)
}

// GenerateSymmetricKey generates a 64-byte symmetric key.
func GenerateSymmetricKey() ([]byte, error) {
	sharedKey := make([]byte, 64)
	_, err := rand.Read(sharedKey)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate symmetric key: %w`, err)
	}
	return sharedKey, nil
}

// GenerateSymmetricJwk generates a symmetric key JWK.
func GenerateSymmetricJwk() (jwk.Key, error) {
	key, err := GenerateSymmetricKey()
	if err != nil {
		return nil, fmt.Errorf(`failed to generate symmetric key: %w`, err)
	}

	k, err := jwk.FromRaw(key)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate jwk.SymmetricKey: %w`, err)
	}

	return k, nil
}

// GenerateEd25519Key generates an Ed25519 private key for signing.
func GenerateEd25519Key() (ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate Ed25519 key: %w`, err)
	}
	return priv, nil
}

// GenerateEd25519Jwk generates an Ed25519 JWK.
func GenerateEd25519Jwk() (jwk.Key, error) {
	key, err := GenerateEd25519Key()
	if err != nil {
		return nil, fmt.Errorf(`failed to generate Ed25519 private key: %w`, err)
	}

	k, err := jwk.FromRaw(key)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate jwk.OKPPrivateKey: %w`, err)
	}

	return k, nil
}

// GenerateX25519Key generates an X25519 private key for key exchange.
func GenerateX25519Key() (x25519.PrivateKey, error) {
	_, priv, err := x25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate X25519 key: %w`, err)
	}
	return priv, nil
}

// GenerateX25519Jwk generates an X25519 JWK.
func GenerateX25519Jwk() (jwk.Key, error) {
	key, err := GenerateX25519Key()
	if err != nil {
		return nil, fmt.Errorf(`failed to generate X25519 private key: %w`, err)
	}

	k, err := jwk.FromRaw(key)
	if err != nil {
		return nil, fmt.Errorf(`failed to generate jwk.OKPPrivateKey: %w`, err)
	}

	return k, nil
}
