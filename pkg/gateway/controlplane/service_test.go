package controlplane

import (
	"reflect"
	"testing"
	"time"
)

func TestConnectAckClonesMethods(t *testing.T) {
	methods := []string{"connect", "ping"}
	service := NewService("openclaw.ws.v1", methods)
	methods[0] = "mutated"

	ack := service.ConnectAck(time.Date(2026, 4, 21, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60)), map[string]any{"id": "u1"})

	if ack.Protocol != "openclaw.ws.v1" {
		t.Fatalf("protocol = %q, want openclaw.ws.v1", ack.Protocol)
	}
	if ack.ConnectedAt != "2026-04-21T01:30:00Z" {
		t.Fatalf("connected_at = %q, want UTC RFC3339", ack.ConnectedAt)
	}
	if !reflect.DeepEqual(ack.Methods, []string{"connect", "ping"}) {
		t.Fatalf("methods = %#v, want cloned original methods", ack.Methods)
	}

	ack.Methods[0] = "changed"
	next := service.ConnectAck(time.Unix(0, 0), nil)
	if !reflect.DeepEqual(next.Methods, []string{"connect", "ping"}) {
		t.Fatalf("service methods were mutated through response: %#v", next.Methods)
	}
}

func TestMethodsListClonesMethods(t *testing.T) {
	service := NewService("openclaw.ws.v1", []string{"connect", "ping"})

	catalog := service.MethodsList()
	catalog.Methods[0] = "changed"

	next := service.MethodsList()
	if !reflect.DeepEqual(next.Methods, []string{"connect", "ping"}) {
		t.Fatalf("methods were mutated through response: %#v", next.Methods)
	}
}

func TestPingFormatsUTC(t *testing.T) {
	service := NewService("openclaw.ws.v1", nil)
	got := service.Ping(time.Date(2026, 4, 21, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60)))

	if got.Pong != "2026-04-21T01:30:00Z" {
		t.Fatalf("pong = %q, want UTC RFC3339", got.Pong)
	}
}

func TestSubscriptionPayload(t *testing.T) {
	service := NewService("openclaw.ws.v1", nil)

	if !service.Subscription(true).Subscribed {
		t.Fatal("Subscription(true).Subscribed = false, want true")
	}
	if service.Subscription(false).Subscribed {
		t.Fatal("Subscription(false).Subscribed = true, want false")
	}
}
