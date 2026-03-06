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

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type singleNodeStrategy struct {
	client redis.UniversalClient
}

func (s *singleNodeStrategy) tryAcquire(ctx context.Context, lease *Lease) error {
	obtained, err := s.client.SetNX(ctx, lease.fullKey, lease.token, lease.ttl).Result()
	if err != nil {
		return err
	}
	if !obtained {
		return ErrNotObtained
	}
	return nil
}

func (s *singleNodeStrategy) release(ctx context.Context, lease *Lease) error {
	deleted, err := releaseScript.Run(ctx, s.client, []string{lease.fullKey}, lease.token).Int64()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrLockNotHeld
	}
	return nil
}

func (s *singleNodeStrategy) refresh(ctx context.Context, lease *Lease) error {
	extended, err := refreshScript.Run(
		ctx,
		s.client,
		[]string{lease.fullKey},
		lease.token,
		lease.ttl.Milliseconds(),
	).Int64()
	if err != nil {
		return err
	}
	if extended == 0 {
		return ErrLockNotHeld
	}
	return nil
}
