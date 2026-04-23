package events

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBusPublishAppliesMiddlewareAndReturnsLastError(t *testing.T) {
	bus := NewEventBus()
	var calls []string
	firstErr := errors.New("first")
	lastErr := errors.New("last")

	bus.Use(func(next EventHandler) EventHandler {
		return func(ctx context.Context, event Event) error {
			calls = append(calls, "before")
			err := next(ctx, event)
			calls = append(calls, "after")
			return err
		}
	})
	bus.Subscribe(EventGatewayStart, func(ctx context.Context, event Event) error {
		calls = append(calls, "handler1:"+event.Source)
		return firstErr
	})
	bus.Subscribe(EventGatewayStart, func(ctx context.Context, event Event) error {
		calls = append(calls, "handler2:"+event.Source)
		return lastErr
	})

	err := bus.Publish(context.Background(), Event{
		Type:      EventGatewayStart,
		Source:    "gateway",
		Data:      map[string]interface{}{"ready": true},
		Timestamp: 42,
	})
	if !errors.Is(err, lastErr) {
		t.Fatalf("expected last handler error, got %v", err)
	}

	wantCalls := []string{"before", "handler1:gateway", "handler2:gateway", "after"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected call order: %#v", calls)
	}

	history := bus.GetHistory(1)
	if len(history) != 1 || history[0].Type != EventGatewayStart {
		t.Fatalf("expected published event in history, got %#v", history)
	}
}

func TestEventBusSubscribeAllHistoryAndClear(t *testing.T) {
	bus := NewEventBus()
	bus.maxHistory = 2

	var count int32
	bus.SubscribeAll(func(ctx context.Context, event Event) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	if err := bus.Publish(context.Background(), Event{Type: EventAgentStart, Source: "agent"}); err != nil {
		t.Fatalf("publish agent: %v", err)
	}
	if err := bus.Publish(context.Background(), Event{Type: EventToolCall, Source: "tool"}); err != nil {
		t.Fatalf("publish tool: %v", err)
	}
	if err := bus.Publish(context.Background(), Event{Type: EventGatewayStop, Source: "gateway"}); err != nil {
		t.Fatalf("publish gateway: %v", err)
	}

	if got := atomic.LoadInt32(&count); got != 3 {
		t.Fatalf("expected SubscribeAll handler to run 3 times, got %d", got)
	}

	history := bus.GetHistory(0)
	if len(history) != 2 || history[0].Type != EventToolCall || history[1].Type != EventGatewayStop {
		t.Fatalf("expected trimmed history with last two events, got %#v", history)
	}

	subscriptions := bus.GetSubscriptions()
	if subscriptions[EventAgentStart] != 1 || subscriptions[EventGatewayStop] != 1 {
		t.Fatalf("expected SubscribeAll subscriptions, got %#v", subscriptions)
	}

	bus.Clear()
	if got := bus.GetSubscriptions(); len(got) != 0 {
		t.Fatalf("expected cleared subscriptions, got %#v", got)
	}
	if err := bus.Publish(context.Background(), Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("publish without handlers: %v", err)
	}
}

func TestEventBusPublishAsync(t *testing.T) {
	bus := NewEventBus()
	done := make(chan struct{}, 1)

	bus.Subscribe(EventChannelMessage, func(ctx context.Context, event Event) error {
		if event.Source != "telegram" {
			t.Fatalf("expected telegram source, got %q", event.Source)
		}
		done <- struct{}{}
		return nil
	})

	bus.PublishAsync(context.Background(), Event{Type: EventChannelMessage, Source: "telegram"})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async publish")
	}
}
