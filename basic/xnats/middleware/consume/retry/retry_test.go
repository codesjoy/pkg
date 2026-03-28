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

package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

func TestDropPolicyAcksJetStreamMessage(t *testing.T) {
	acker := &fakeAcker{}
	middleware := New(Config{MaxRetries: 0}, ExhaustedPolicyDrop, nil, nil)

	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Transport: consume.TransportJetStream,
		Acker:     acker,
	}, func(context.Context, *consume.MessageContext) error {
		return errors.New("boom")
	})

	require.NoError(t, err)
	require.True(t, acker.acked)
	require.False(t, acker.nacked)
}

func TestStopPolicyNaksJetStreamMessage(t *testing.T) {
	acker := &fakeAcker{}
	middleware := New(Config{MaxRetries: 0}, ExhaustedPolicyStop, nil, nil)

	err := middleware.Handle(context.Background(), &consume.MessageContext{
		Transport: consume.TransportJetStream,
		Acker:     acker,
	}, func(context.Context, *consume.MessageContext) error {
		return errors.New("boom")
	})

	require.Error(t, err)
	require.True(t, acker.nacked)
	require.False(t, acker.acked)
}

type fakeAcker struct {
	acked  bool
	nacked bool
}

func (a *fakeAcker) Ack() error {
	a.acked = true
	return nil
}

func (a *fakeAcker) Nak() error {
	a.nacked = true
	return nil
}

func (a *fakeAcker) Handled() bool {
	return a.acked || a.nacked
}
