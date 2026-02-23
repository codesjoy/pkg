# xjwt

JWT key generation utilities for Go.

## Overview

The `xjwt` package provides a comprehensive set of functions for generating cryptographic keys used in JWT (JSON Web Token) operations. It supports multiple key types including RSA, ECDSA, Ed25519, X25519, and symmetric keys, with full JWK (JSON Web Key) format support.

## Features

- **RSA**: Generate 2048-bit RSA keys
- **ECDSA**: Support for P-256, P-384, and P-521 elliptic curves
- **Ed25519**: EdDSA signing keys for modern cryptographic applications
- **X25519**: ECDH key exchange keys for secure key exchange
- **Symmetric**: 64-byte symmetric keys for HMAC-based algorithms
- **JWK**: Full JSON Web Key format generation and serialization
- **Public Key Extraction**: Extract public keys from private key pairs

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xjwt
```

## Usage

### Generate RSA Key

Generate a 2048-bit RSA private key:

```go
package main

import (
    "fmt"
    "log"

    "github.com/codesjoy/pkg/basic/xjwt"
)

func main() {
    key, err := xjwt.GenerateRsaKey()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("RSA Key: %v\n", key)
}
```

### Generate RSA JWK

Generate an RSA key in JWK format:

```go
jwkKey, err := xjwt.GenerateRsaJwk()
if err != nil {
    log.Fatal(err)
}

// Serialize to JSON
jsonBytes, err := json.MarshalIndent(jwkKey, "", "  ")
if err != nil {
    log.Fatal(err)
}
fmt.Println(string(jsonBytes))
```

### Generate Public JWK from RSA

Extract the public key from an RSA private key:

```go
publicJwk, err := xjwt.GenerateRsaPublicJwk()
if err != nil {
    log.Fatal(err)
}
```

### Generate ECDSA Keys

Generate ECDSA keys for different elliptic curves:

```go
import "github.com/lestrrat-go/jwx/v2/jwa"

// Using specific curve functions
p256Key, err := xjwt.GenerateEcdsaJwkP256()
if err != nil {
    log.Fatal(err)
}

p384Key, err := xjwt.GenerateEcdsaJwkP384()
if err != nil {
    log.Fatal(err)
}

p521Key, err := xjwt.GenerateEcdsaJwkP521()
if err != nil {
    log.Fatal(err)
}

// Or using the generic function with algorithm parameter
key, err := xjwt.GenerateEcdsaJwk(jwa.P256)
if err != nil {
    log.Fatal(err)
}
```

### Generate ECDSA Public JWK

Extract the public key from an ECDSA private key:

```go
publicJwk, err := xjwt.GenerateEcdsaPublicJwk(jwa.P256)
if err != nil {
    log.Fatal(err)
}
```

### Generate Ed25519 Key

Generate an Ed25519 key for signing:

```go
key, err := xjwt.GenerateEd25519Key()
if err != nil {
    log.Fatal(err)
}

jwkKey, err := xjwt.GenerateEd25519Jwk()
if err != nil {
    log.Fatal(err)
}
```

### Generate X25519 Key

Generate an X25519 key for key exchange:

```go
key, err := xjwt.GenerateX25519Key()
if err != nil {
    log.Fatal(err)
}

jwkKey, err := xjwt.GenerateX25519Jwk()
if err != nil {
    log.Fatal(err)
}
```

### Generate Symmetric Key

Generate a 64-byte symmetric key:

```go
key, err := xjwt.GenerateSymmetricKey()
if err != nil {
    log.Fatal(err)
}

jwkKey, err := xjwt.GenerateSymmetricJwk()
if err != nil {
    log.Fatal(err)
}
```

## API Reference

### RSA Functions

| Function | Description |
|----------|-------------|
| `GenerateRsaKey()` | Generates a 2048-bit RSA private key |
| `GenerateRsaJwk()` | Generates an RSA JWK (JSON Web Key) |
| `GenerateRsaPublicJwk()` | Generates a public JWK from an RSA private key |

### ECDSA Functions

| Function | Description |
|----------|-------------|
| `GenerateEcdsaKey(alg)` | Generates an ECDSA private key for the specified curve |
| `GenerateEcdsaJwk(alg)` | Generates an ECDSA JWK for the specified curve |
| `GenerateEcdsaJwkP256()` | Generates a P256 ECDSA JWK |
| `GenerateEcdsaJwkP384()` | Generates a P384 ECDSA JWK |
| `GenerateEcdsaJwkP521()` | Generates a P521 ECDSA JWK |
| `GenerateEcdsaPublicJwk(alg)` | Generates a public JWK from an ECDSA private key |

### Ed25519 Functions

| Function | Description |
|----------|-------------|
| `GenerateEd25519Key()` | Generates an Ed25519 private key for signing |
| `GenerateEd25519Jwk()` | Generates an Ed25519 JWK |

### X25519 Functions

| Function | Description |
|----------|-------------|
| `GenerateX25519Key()` | Generates an X25519 private key for key exchange |
| `GenerateX25519Jwk()` | Generates an X25519 JWK |

### Symmetric Key Functions

| Function | Description |
|----------|-------------|
| `GenerateSymmetricKey()` | Generates a 64-byte symmetric key |
| `GenerateSymmetricJwk()` | Generates a symmetric key JWK |

### Elliptic Curve Utilities

| Function | Description |
|----------|-------------|
| `RegisterCurve(crv, alg)` | Registers a curve for use with JWT |
| `IsAvailable(alg)` | Returns true if the given algorithm is available |
| `AvailableAlgorithms()` | Returns the list of available algorithms |
| `AvailableCurves()` | Returns the list of available curves |
| `AlgorithmForCurve(crv)` | Returns the algorithm for the given curve |
| `CurveForAlgorithm(alg)` | Returns the curve for the given algorithm |

## Dependencies

- [`github.com/lestrrat-go/jwx/v2`](https://github.com/lestrrat-go/jwx) - JWK/JWA implementation
- Go standard library crypto packages

## License

Copyright 2022 The codesjoy Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
