package state

import "testing"

func TestEventBusSubscribeUsesDefaultBufferAndUnsubscribeStopsDelivery(t *testing.T) {
	bus := NewEventBus()

	sub := bus.Subscribe(0)
	if cap(sub) != 16 {
		t.Fatalf("expected default subscriber buffer 16, got %d", cap(sub))
	}

	bus.Publish(NewEvent("created", "sess-1", map[string]any{"count": 1}))
	select {
	case event, ok := <-sub:
		if !ok {
			t.Fatal("expected open subscriber channel before unsubscribe")
		}
		if event == nil || event.Type != "created" {
			t.Fatalf("expected delivered event, got %#v", event)
		}
	default:
		t.Fatal("expected subscriber to receive published event")
	}

	bus.Unsubscribe(sub)
	select {
	case _, ok := <-sub:
		if ok {
			t.Fatal("expected unsubscribed channel to be closed")
		}
	default:
		t.Fatal("expected unsubscribe to close channel immediately")
	}

	bus.Publish(NewEvent("ignored", "sess-1", nil))
}

func TestEventBusPublishSkipsBlockedSubscribersAndClonesEvents(t *testing.T) {
	bus := NewEventBus()

	blocked := bus.Subscribe(1)
	ready := bus.Subscribe(1)
	blocked <- NewEvent("prefill", "sess-1", nil)

	original := NewEvent("updated", "sess-1", map[string]any{"count": 1})
	bus.Publish(original)

	select {
	case event := <-ready:
		if event == original {
			t.Fatal("expected published event to be cloned")
		}
		event.Payload["count"] = 99
	default:
		t.Fatal("expected ready subscriber to receive event even if another subscriber is full")
	}

	if original.Payload["count"] != 1 {
		t.Fatalf("expected original event payload to remain unchanged, got %#v", original.Payload)
	}

	bus.Publish(original)
	select {
	case event := <-ready:
		if event.Payload["count"] != 1 {
			t.Fatalf("expected fresh clone on each publish, got %#v", event.Payload)
		}
	default:
		t.Fatal("expected second publish to deliver event")
	}
}

func TestEventBusPublishIgnoresNilInputs(t *testing.T) {
	var nilBus *EventBus
	nilBus.Publish(NewEvent("ignored", "sess-1", nil))

	bus := NewEventBus()
	bus.Publish(nil)
}
