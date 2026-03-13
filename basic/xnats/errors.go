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

package xnats

import "errors"

var (
	// ErrNilPublishMessage indicates no message was supplied for publish.
	ErrNilPublishMessage = errors.New("publish message is nil")
	// ErrPublishSubjectRequired indicates a subject is required before publish.
	ErrPublishSubjectRequired = errors.New("publish subject is required")
	// ErrRequesterClosed indicates requester has been closed.
	ErrRequesterClosed = errors.New("requester is closed")
	// ErrPublisherClosed indicates publisher has been closed.
	ErrPublisherClosed = errors.New("publisher is closed")
	// ErrJetStreamPublisherClosed indicates JetStream publisher has been closed.
	ErrJetStreamPublisherClosed = errors.New("jetstream publisher is closed")
	// ErrSubscriberClosed indicates subscriber has been closed.
	ErrSubscriberClosed = errors.New("subscriber is closed")
	// ErrJetStreamConsumerClosed indicates JetStream consumer has been closed.
	ErrJetStreamConsumerClosed = errors.New("jetstream consumer is closed")
	// ErrSubscriberActive indicates one subscriber Consume call is already running.
	ErrSubscriberActive = errors.New("subscriber consume is already running")
	// ErrJetStreamConsumerActive indicates one consumer loop is already running.
	ErrJetStreamConsumerActive = errors.New("jetstream consumer is already running")
	// ErrJetStreamRequired indicates JetStream context is required.
	ErrJetStreamRequired = errors.New("jetstream context is required")
	// ErrPushConsumerRequiresDeliverSubject indicates the bound consumer is not push based.
	ErrPushConsumerRequiresDeliverSubject = errors.New("push consumer requires deliver subject")
	// ErrPullConsumerRequiresNoDeliverSubject indicates the bound consumer is not pull based.
	ErrPullConsumerRequiresNoDeliverSubject = errors.New(
		"pull consumer requires no deliver subject",
	)
)
