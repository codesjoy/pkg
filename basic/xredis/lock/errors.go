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

package lock

import "errors"

var (
	// ErrNilClient indicates the redis client dependency was not provided.
	ErrNilClient = errors.New("redis client is nil")
	// ErrEmptyKey indicates the caller did not provide a lock key.
	ErrEmptyKey = errors.New("lock key is empty")
	// ErrInvalidTTL indicates the lock ttl is not greater than zero.
	ErrInvalidTTL = errors.New("lock ttl must be greater than 0")
	// ErrInvalidRetryInterval indicates the retry interval is negative.
	ErrInvalidRetryInterval = errors.New("lock retry interval must be non-negative")
	// ErrInvalidKeepAliveInterval indicates the keepalive interval is outside the valid ttl window.
	ErrInvalidKeepAliveInterval = errors.New(
		"keepalive interval must be greater than 0 and less than ttl",
	)
	// ErrNotObtained indicates the lock could not be acquired.
	ErrNotObtained = errors.New("lock not obtained")
	// ErrLockNotHeld indicates the caller tried to operate on a lock it does not hold.
	ErrLockNotHeld = errors.New("lock not held")
)
