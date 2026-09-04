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

package main

import (
	"fmt"
	"sort"
	"strings"

	eventv1 "github.com/codesjoy/pkg/proto/codesjoy/ddd/event/v1"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	contextPackage = protogen.GoImportPath("context")
	protoPackage   = protogen.GoImportPath("google.golang.org/protobuf/proto")
	xeventPackage  = protogen.GoImportPath("github.com/codesjoy/pkg/basic/xevent")
)

type fileEvents struct {
	messages []eventMessage
}

type eventMessage struct {
	fullName           string
	goName             string
	eventType          string
	eventIDGetter      string
	partitionKeyGetter string
	topic              string
}

func generateFiles(gen *protogen.Plugin) error {
	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}

		events, err := collectFileEvents(file)
		if err != nil {
			return err
		}
		if !events.hasContent() {
			continue
		}

		g := gen.NewGeneratedFile(
			file.GeneratedFilenamePrefix+"_codesjoy_event.pb.go",
			file.GoImportPath,
		)
		generateHeader(g, file)
		generateFileContent(g, events)
	}
	return nil
}

func collectFileEvents(file *protogen.File) (fileEvents, error) {
	events := fileEvents{}
	for _, message := range file.Messages {
		if err := collectMessageEvents(file, message, &events); err != nil {
			return fileEvents{}, err
		}
	}
	sort.Slice(events.messages, func(i, j int) bool {
		return events.messages[i].fullName < events.messages[j].fullName
	})
	return events, nil
}

func collectMessageEvents(
	file *protogen.File,
	message *protogen.Message,
	events *fileEvents,
) error {
	if message.Desc.IsMapEntry() {
		return nil
	}

	messageEvent, err := describeEventMessage(file, message)
	if err != nil {
		return err
	}
	if messageEvent != nil {
		events.messages = append(events.messages, *messageEvent)
	}

	for _, nested := range message.Messages {
		if err := collectMessageEvents(file, nested, events); err != nil {
			return err
		}
	}
	return nil
}

func describeEventMessage(file *protogen.File, message *protogen.Message) (*eventMessage, error) {
	options, hasEvent, err := messageEventOptions(message)
	if err != nil {
		return nil, wrapMessageError(file, message, err)
	}

	var eventIDGetter string
	var partitionKeyGetter string

	for _, field := range message.Fields {
		hasEventID, err := fieldFlag(field, eventv1.E_EventId)
		if err != nil {
			return nil, wrapFieldError(
				file,
				message,
				field,
				fmt.Errorf("read event_id option: %w", err),
			)
		}
		hasPartitionKey, err := fieldFlag(field, eventv1.E_PartitionKey)
		if err != nil {
			return nil, wrapFieldError(
				file,
				message,
				field,
				fmt.Errorf("read partition_key option: %w", err),
			)
		}
		if !hasEventID && !hasPartitionKey {
			continue
		}
		if !hasEvent {
			return nil, wrapFieldError(
				file,
				message,
				field,
				fmt.Errorf(
					"field annotation requires message option (codesjoy.ddd.event.v1.event)",
				),
			)
		}
		if err := validateAnnotatedField(field); err != nil {
			return nil, wrapFieldError(file, message, field, err)
		}

		getter := "Get" + field.GoName
		if hasEventID {
			if eventIDGetter != "" {
				return nil, wrapFieldError(
					file,
					message,
					field,
					fmt.Errorf("duplicate event_id annotation; already set on another field"),
				)
			}
			eventIDGetter = getter
		}
		if hasPartitionKey {
			if partitionKeyGetter != "" {
				return nil, wrapFieldError(
					file,
					message,
					field,
					fmt.Errorf("duplicate partition_key annotation; already set on another field"),
				)
			}
			partitionKeyGetter = getter
		}
	}

	if !hasEvent {
		return nil, nil
	}

	eventType := string(message.Desc.FullName())
	if options.GetEventType() != "" {
		eventType = options.GetEventType()
	}

	return &eventMessage{
		fullName:           string(message.Desc.FullName()),
		goName:             message.GoIdent.GoName,
		eventType:          eventType,
		eventIDGetter:      eventIDGetter,
		partitionKeyGetter: partitionKeyGetter,
		topic:              options.GetTopic(),
	}, nil
}

func messageEventOptions(message *protogen.Message) (*eventv1.EventOptions, bool, error) {
	if !proto.HasExtension(message.Desc.Options(), eventv1.E_Event) {
		return nil, false, nil
	}

	ext := proto.GetExtension(message.Desc.Options(), eventv1.E_Event)
	switch typed := ext.(type) {
	case *eventv1.EventOptions:
		if typed == nil {
			return &eventv1.EventOptions{}, true, nil
		}
		if typed.GetEventType() != "" && strings.TrimSpace(typed.GetEventType()) == "" {
			return nil, false, fmt.Errorf("event.event_type must not be blank")
		}
		return typed, true, nil
	default:
		return nil, false, fmt.Errorf("unexpected event extension type %T", ext)
	}
}

func fieldFlag(
	field *protogen.Field,
	extension protoreflect.ExtensionType,
) (bool, error) {
	if !proto.HasExtension(field.Desc.Options(), extension) {
		return false, nil
	}

	ext := proto.GetExtension(field.Desc.Options(), extension)
	switch typed := ext.(type) {
	case bool:
		return typed, nil
	case *bool:
		return typed != nil && *typed, nil
	default:
		return false, fmt.Errorf("unexpected bool extension type %T", ext)
	}
}

func validateAnnotatedField(field *protogen.Field) error {
	switch {
	case field.Desc.IsMap():
		return fmt.Errorf("annotation requires a non-map string field")
	case field.Desc.IsList():
		return fmt.Errorf("annotation requires a non-repeated string field")
	case fieldOneof(field) != nil:
		return fmt.Errorf("annotation requires a non-oneof string field")
	case field.Desc.Kind() != protoreflect.StringKind:
		return fmt.Errorf("annotation requires a string field")
	default:
		return nil
	}
}

func fieldOneof(field *protogen.Field) protoreflect.OneofDescriptor {
	oneof := field.Desc.ContainingOneof()
	if oneof == nil || oneof.IsSynthetic() {
		return nil
	}
	return oneof
}

func wrapMessageError(file *protogen.File, message *protogen.Message, err error) error {
	return fmt.Errorf("file %s: message %s: %w", file.Desc.Path(), message.Desc.FullName(), err)
}

func wrapFieldError(
	file *protogen.File,
	message *protogen.Message,
	field *protogen.Field,
	err error,
) error {
	return fmt.Errorf(
		"file %s: message %s field %s: %w",
		file.Desc.Path(),
		message.Desc.FullName(),
		field.Desc.Name(),
		err,
	)
}

func (events fileEvents) hasContent() bool {
	return len(events.messages) > 0
}

func generateHeader(g *protogen.GeneratedFile, file *protogen.File) {
	g.P("// Code generated by protoc-gen-codesjoy-event. DO NOT EDIT.")
	g.P("// source: ", file.Desc.Path())
	g.P()
	g.P("package ", file.GoPackageName)
	g.P()
}

func generateFileContent(g *protogen.GeneratedFile, events fileEvents) {
	for _, message := range events.messages {
		eventTypeMethod := renderStringMethod(message.goName, "EventType", message.eventType)
		eventIDMethod := renderGetterMethod(message.goName, "EventID", message.eventIDGetter)
		partitionKeyMethod := renderGetterMethod(
			message.goName,
			"PartitionKey",
			message.partitionKeyGetter,
		)

		g.P(
			"var _ ",
			g.QualifiedGoIdent(xeventPackage.Ident("Event")),
			" = (*",
			message.goName,
			")(nil)",
		)
		g.P()
		g.P(eventTypeMethod)
		g.P()
		g.P(eventIDMethod)
		g.P()
		g.P(partitionKeyMethod)
		g.P()
		topicMethod := renderStringMethod(message.goName, "Topic", message.topic)
		g.P(topicMethod)
		g.P()
		g.P("func (x *", message.goName, ") MarshalPayload() ([]byte, error) {")
		g.P("\treturn ", g.QualifiedGoIdent(protoPackage.Ident("Marshal")), "(x)")
		g.P("}")
		g.P()
		g.P("func (x *", message.goName, ") UnmarshalPayload(data []byte) error {")
		g.P("\treturn ", g.QualifiedGoIdent(protoPackage.Ident("Unmarshal")), "(data, x)")
		g.P("}")
		g.P()
		g.P("func On", message.goName, "(")
		g.P("\td *", g.QualifiedGoIdent(xeventPackage.Ident("Dispatcher")), ",")
		g.P(
			"\thandler func(",
			g.QualifiedGoIdent(contextPackage.Ident("Context")),
			", *",
			message.goName,
			") error,",
		)
		g.P(") error {")
		g.P(
			"\treturn ",
			g.QualifiedGoIdent(xeventPackage.Ident("On")),
			"[*",
			message.goName,
			"](d, handler)",
		)
		g.P("}")
		g.P()
	}
}

func renderStringMethod(goName string, method string, value string) string {
	return fmt.Sprintf("func (*%s) %s() string {\n\treturn %q\n}", goName, method, value)
}

func renderGetterMethod(goName string, method string, getter string) string {
	if getter == "" {
		return fmt.Sprintf("func (x *%s) %s() string {\n\treturn \"\"\n}", goName, method)
	}
	return fmt.Sprintf(
		"func (x *%s) %s() string {\n\tif x == nil {\n\t\treturn \"\"\n\t}\n\treturn x.%s()\n}",
		goName,
		method,
		getter,
	)
}
