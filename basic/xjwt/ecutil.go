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

// Package xjwt defines tools that help with elliptic curve related
// computation
package xjwt

import (
	"crypto/elliptic"
	"math/big"
	"sync"

	"github.com/lestrrat-go/jwx/v2/jwa"
)

// data for available curves. Some algorithms may be compiled in/out
var (
	curveToAlg    = map[elliptic.Curve]jwa.EllipticCurveAlgorithm{}
	algToCurve    = map[jwa.EllipticCurveAlgorithm]elliptic.Curve{}
	availableAlgs []jwa.EllipticCurveAlgorithm
	availableCrvs []elliptic.Curve
)

func init() {
	RegisterCurve(elliptic.P256(), jwa.P256)
	RegisterCurve(elliptic.P384(), jwa.P384)
	RegisterCurve(elliptic.P521(), jwa.P521)
}

// RegisterCurve registers a curve for use with JWT
func RegisterCurve(crv elliptic.Curve, alg jwa.EllipticCurveAlgorithm) {
	curveToAlg[crv] = alg
	algToCurve[alg] = crv
	availableAlgs = append(availableAlgs, alg)
	availableCrvs = append(availableCrvs, crv)
}

// IsAvailable returns true if the given algorithm is available
func IsAvailable(alg jwa.EllipticCurveAlgorithm) bool {
	_, ok := algToCurve[alg]
	return ok
}

// AvailableAlgorithms returns the list of available algorithms
func AvailableAlgorithms() []jwa.EllipticCurveAlgorithm {
	return availableAlgs
}

// AvailableCurves returns the list of available curves
func AvailableCurves() []elliptic.Curve {
	return availableCrvs
}

// AlgorithmForCurve returns the algorithm for the given curve
func AlgorithmForCurve(crv elliptic.Curve) (jwa.EllipticCurveAlgorithm, bool) {
	v, ok := curveToAlg[crv]
	return v, ok
}

// CurveForAlgorithm returns the curve for the given algorithm
func CurveForAlgorithm(alg jwa.EllipticCurveAlgorithm) (elliptic.Curve, bool) {
	v, ok := algToCurve[alg]
	return v, ok
}

const (
	// size of buffer that needs to be allocated for EC521 curve
	ec521BufferSize = 66 // (521 / 8) + 1
)

var ecpointBufferPool = sync.Pool{
	New: func() interface{} {
		// In most cases the curve bit size will be less than this length
		// so allocate the maximum, and keep reusing
		buf := make([]byte, 0, ec521BufferSize)
		return &buf
	},
}

func getCrvFixedBuffer(size int) []byte {
	//nolint:forcetypeassert
	buf := *(ecpointBufferPool.Get().(*[]byte))
	if size > ec521BufferSize && cap(buf) < size {
		buf = append(buf, make([]byte, size-cap(buf))...)
	}
	return buf[:size]
}

// ReleaseECPointBuffer releases the []byte buffer allocated.
func ReleaseECPointBuffer(buf []byte) {
	buf = buf[:cap(buf)]
	buf[0] = 0x0
	for i := 1; i < len(buf); i *= 2 {
		copy(buf[i:], buf[:i])
	}
	buf = buf[:0]
	ecpointBufferPool.Put(&buf)
}

// AllocECPointBuffer allocates a buffer for the given point in the given
// curve. This buffer should be released using the ReleaseECPointBuffer
// function.
func AllocECPointBuffer(v *big.Int, crv elliptic.Curve) []byte {
	// We need to create a buffer that fits the entire curve.
	// If the curve size is 66, that fits in 9 bytes. If the curve
	// size is 64, it fits in 8 bytes.
	bits := crv.Params().BitSize

	// For most common cases we know before hand what the byte length
	// is going to be. optimize
	var inBytes int
	switch bits {
	// TODO: use constant?
	// nolint: mnd
	case 224, 256, 384:
		inBytes = bits / 8
	case 521: //nolint: mnd
		inBytes = ec521BufferSize
	default:
		inBytes = bits / 8
		if (bits % 8) != 0 {
			inBytes++
		}
	}

	buf := getCrvFixedBuffer(inBytes)
	v.FillBytes(buf)
	return buf
}
