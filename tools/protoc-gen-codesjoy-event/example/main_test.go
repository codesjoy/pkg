package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/codesjoy/pkg/basic/xevent"
	orderv1 "github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-event/example/protogen/codesjoy/example/order/v1"
)

func TestGeneratedMessageDispatchRoundTrip(t *testing.T) {
	dispatcher := xevent.NewDispatcher()

	var got *orderv1.OrderCreated
	if err := orderv1.OnOrderCreated(
		dispatcher,
		func(_ context.Context, event *orderv1.OrderCreated) error {
			got = event
			return nil
		},
	); err != nil {
		t.Fatalf("On() error = %v", err)
	}

	input := &orderv1.OrderCreated{
		Id:      "evt_1",
		OrderId: "order-1",
		UserId:  "u_1",
	}
	payload, err := input.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload() error = %v", err)
	}

	if err := dispatcher.Handle(context.Background(), &xevent.Message{
		EventType: input.EventType(),
		Payload:   payload,
	}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if got == nil {
		t.Fatal("expected typed event")
	}
	if got.EventType() != "codesjoy.example.order.v1.OrderCreated" {
		t.Fatalf(
			"EventType() = %q, want %q",
			got.EventType(),
			"codesjoy.example.order.v1.OrderCreated",
		)
	}
	if got.EventID() != "evt_1" {
		t.Fatalf("EventID() = %q, want %q", got.EventID(), "evt_1")
	}
	if got.PartitionKey() != "order-1" {
		t.Fatalf("PartitionKey() = %q, want %q", got.PartitionKey(), "order-1")
	}
	if got.GetUserId() != "u_1" {
		t.Fatalf("GetUserId() = %q, want %q", got.GetUserId(), "u_1")
	}
}

func TestGeneratedMessageHelperRegistersMultipleHandlersInOrder(t *testing.T) {
	dispatcher := xevent.NewDispatcher()

	var calls []string
	if err := orderv1.OnOrderCreated(
		dispatcher,
		func(_ context.Context, event *orderv1.OrderCreated) error {
			calls = append(calls, "first:"+event.GetUserId())
			return nil
		},
		func(_ context.Context, event *orderv1.OrderCreated) error {
			calls = append(calls, "second:"+event.GetUserId())
			return nil
		},
	); err != nil {
		t.Fatalf("OnOrderCreated() error = %v", err)
	}

	input := &orderv1.OrderCreated{
		Id:      "evt_2",
		OrderId: "order-2",
		UserId:  "u_2",
	}
	payload, err := input.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload() error = %v", err)
	}

	if err := dispatcher.Handle(context.Background(), &xevent.Message{
		EventType: input.EventType(),
		Payload:   payload,
	}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if got := len(calls); got != 2 {
		t.Fatalf("handler call count = %d, want 2", got)
	}
	if calls[0] != "first:u_2" || calls[1] != "second:u_2" {
		t.Fatalf("handler order = %v, want [first:u_2 second:u_2]", calls)
	}
}

func TestGeneratedMessageHelperRejectsEmptyHandlers(t *testing.T) {
	dispatcher := xevent.NewDispatcher()

	err := orderv1.OnOrderCreated(dispatcher)
	if !errors.Is(err, xevent.ErrInvalidEventBinding) {
		t.Fatalf("OnOrderCreated() error = %v, want ErrInvalidEventBinding", err)
	}
}

func TestGeneratedMessageOptionalMetadataDefaultsEmpty(t *testing.T) {
	event := &orderv1.AuditPing{Actor: "system"}
	if got := event.EventType(); got != "audit.ping" {
		t.Fatalf("EventType() = %q, want %q", got, "audit.ping")
	}
	if got := event.EventID(); got != "" {
		t.Fatalf("EventID() = %q, want empty", got)
	}
	if got := event.PartitionKey(); got != "" {
		t.Fatalf("PartitionKey() = %q, want empty", got)
	}
}

func TestBufGenerateMatchesCommittedOutput(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	exampleDir := filepath.Dir(currentFile)
	toolDir := filepath.Dir(exampleDir)
	repoRoot := filepath.Dir(filepath.Dir(toolDir))
	tmpRoot := t.TempDir()
	tmpToolDir := filepath.Join(tmpRoot, "tools", "protoc-gen-codesjoy-event")

	if err := copyDir(toolDir, tmpToolDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}
	if err := copyDir(filepath.Join(repoRoot, "proto"), filepath.Join(tmpRoot, "proto")); err != nil {
		t.Fatalf("copyDir(proto) error = %v", err)
	}
	if err := copyDir(filepath.Join(repoRoot, "basic", "xevent"), filepath.Join(tmpRoot, "basic", "xevent")); err != nil {
		t.Fatalf("copyDir(xevent) error = %v", err)
	}

	cmd := exec.Command("buf", "generate")
	cmd.Dir = filepath.Join(tmpToolDir, "example")
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("buf generate error = %v\n%s", err, output)
	}

	assertSameFile(
		t,
		filepath.Join(exampleDir, "protogen/codesjoy/example/order/v1/order.pb.go"),
		filepath.Join(tmpToolDir, "example/protogen/codesjoy/example/order/v1/order.pb.go"),
	)
	assertSameFile(
		t,
		filepath.Join(exampleDir, "protogen/codesjoy/example/order/v1/order_codesjoy_event.pb.go"),
		filepath.Join(tmpToolDir, "example/protogen/codesjoy/example/order/v1/order_codesjoy_event.pb.go"),
	)
}

func assertSameFile(t *testing.T, wantPath string, gotPath string) {
	t.Helper()

	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", wantPath, err)
	}
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", gotPath, err)
	}
	if string(got) != string(want) {
		t.Fatalf("generated file mismatch: %s", filepath.Base(wantPath))
	}
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
