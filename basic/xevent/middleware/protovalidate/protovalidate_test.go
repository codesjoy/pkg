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

package protovalidate

import (
	"context"
	"errors"
	"testing"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	bufprotovalidate "buf.build/go/protovalidate"
	"github.com/codesjoy/pkg/basic/xevent"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type testProtoEvent struct {
	*dynamicpb.Message
}

func (*testProtoEvent) EventType() string                 { return "test.event" }
func (*testProtoEvent) EventID() string                   { return "evt-1" }
func (*testProtoEvent) PartitionKey() string              { return "" }
func (*testProtoEvent) Topic() string                     { return "" }
func (e *testProtoEvent) MarshalPayload() ([]byte, error) { return proto.Marshal(e.Message) }
func (e *testProtoEvent) UnmarshalPayload(data []byte) error {
	return proto.Unmarshal(data, e.Message)
}

type nonProtoEvent struct{}

func (*nonProtoEvent) EventType() string               { return "non-proto.event" }
func (*nonProtoEvent) EventID() string                 { return "evt-2" }
func (*nonProtoEvent) PartitionKey() string            { return "" }
func (*nonProtoEvent) Topic() string                   { return "" }
func (*nonProtoEvent) MarshalPayload() ([]byte, error) { return nil, nil }
func (*nonProtoEvent) UnmarshalPayload([]byte) error   { return nil }

func TestMiddlewareValidProtoEventCallsNext(t *testing.T) {
	event, field := newTestProtoEvent(t)
	event.Set(field, protoreflect.ValueOfString("order-1"))
	assertCallsNext(t, Config{}, event)
}

func TestMiddlewareInvalidProtoEventStopsChain(t *testing.T) {
	event, _ := newTestProtoEvent(t)
	called := false
	middleware := newMiddleware(t, Config{})
	err := middleware.Handle(
		context.Background(),
		&xevent.EventContext{Event: event},
		func(context.Context, *xevent.EventContext) error {
			called = true
			return nil
		},
	)
	if !errors.Is(err, xevent.ErrDiscard) {
		t.Fatalf("expected ErrDiscard, got %v", err)
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
	var validationErr *bufprotovalidate.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected protovalidate.ValidationError, got %T", err)
	}
	if called {
		t.Fatal("expected validation failure to stop the chain")
	}
}

func TestMiddlewareNonProtoEventCallsNext(t *testing.T) {
	assertCallsNext(t, Config{}, &nonProtoEvent{})
}

func TestMiddlewareRejectsNilEvent(t *testing.T) {
	middleware := newMiddleware(t, Config{})
	for _, eventCtx := range []*xevent.EventContext{nil, {}} {
		err := middleware.Handle(
			context.Background(),
			eventCtx,
			func(context.Context, *xevent.EventContext) error { return nil },
		)
		if !errors.Is(err, xevent.ErrNilEvent) {
			t.Fatalf("expected ErrNilEvent, got %v", err)
		}
	}
}

func TestNewAppliesValidatorOptions(t *testing.T) {
	event, field := newTestProtoEvent(t)
	middleware := newMiddleware(t, Config{ValidatorOptions: []bufprotovalidate.ValidatorOption{
		bufprotovalidate.WithMessages(event),
		bufprotovalidate.WithDisableLazy(),
	}})
	event.Set(field, protoreflect.ValueOfString("order-1"))
	called := false
	if err := middleware.Handle(context.Background(), &xevent.EventContext{Event: event}, func(context.Context, *xevent.EventContext) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if !called {
		t.Fatal("expected next to be called")
	}

	unknownEvent, unknownField := newTestProtoEvent(t)
	unknownEvent.Set(unknownField, protoreflect.ValueOfString("order-2"))
	err := middleware.Handle(context.Background(), &xevent.EventContext{Event: unknownEvent}, func(context.Context, *xevent.EventContext) error {
		t.Fatal("expected unknown descriptor to fail with lazy validation disabled")
		return nil
	})
	var compilationErr *bufprotovalidate.CompilationError
	if !errors.As(err, &compilationErr) {
		t.Fatalf("expected CompilationError for unknown descriptor, got %T: %v", err, err)
	}
}

func newMiddleware(t *testing.T, cfg Config) *Middleware {
	t.Helper()
	middleware, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return middleware
}

func assertCallsNext(t *testing.T, cfg Config, event xevent.Event) {
	t.Helper()
	called := false
	middleware := newMiddleware(t, cfg)
	err := middleware.Handle(
		context.Background(),
		&xevent.EventContext{Event: event},
		func(context.Context, *xevent.EventContext) error {
			called = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if !called {
		t.Fatal("expected next to be called")
	}
}

func newTestProtoEvent(t *testing.T) (*testProtoEvent, protoreflect.FieldDescriptor) {
	t.Helper()
	options := &descriptorpb.FieldOptions{}
	proto.SetExtension(options, validatepb.E_Field, validatepb.FieldRules_builder{
		String: validatepb.StringRules_builder{MinLen: proto.Uint64(1)}.Build(),
	}.Build())
	descriptor, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Syntax:  proto.String("proto3"),
		Name:    proto.String("test/event.proto"),
		Package: proto.String("codesjoy.test"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Event"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:    proto.String("id"),
				Number:  proto.Int32(1),
				Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:    descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Options: options,
			}},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("protodesc.NewFile returned error: %v", err)
	}
	messageDescriptor := descriptor.Messages().ByName("Event")
	field := messageDescriptor.Fields().ByName("id")
	return &testProtoEvent{Message: dynamicpb.NewMessage(messageDescriptor)}, field
}
