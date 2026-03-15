package main

import (
	"context"
	"fmt"

	"github.com/codesjoy/pkg/basic/xevent"
	orderv1 "github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-ddd/example/protogen/codesjoy/example/order/v1"
)

func main() {
	dispatcher := xevent.NewDispatcher()

	_ = orderv1.OnOrderCreated(
		dispatcher,
		func(_ context.Context, event *orderv1.OrderCreated) error {
			fmt.Printf(
				"type=%s id=%s key=%s user=%s\n",
				event.EventType(),
				event.EventID(),
				event.PartitionKey(),
				event.GetUserId(),
			)
			return nil
		},
	)

	event := &orderv1.OrderCreated{
		Id:      "evt_1",
		OrderId: "order-1",
		UserId:  "u_1",
	}
	payload, _ := event.MarshalPayload()
	_ = dispatcher.Handle(context.Background(), &xevent.Message{
		EventType: event.EventType(),
		Payload:   payload,
	})
}
