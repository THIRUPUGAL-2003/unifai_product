package handlers

import (
	"testing"
	"time"
)

func TestGuardEventHubNotifyTargetsOneAgent(t *testing.T) {
	hub := &guardEventHub{waiters: map[string]map[chan string]struct{}{}}
	a := hub.subscribe("agent-a")
	b := hub.subscribe("agent-b")
	defer hub.unsubscribe("agent-a", a)
	defer hub.unsubscribe("agent-b", b)

	hub.notify("agent-a", guardEventUninstall)

	select {
	case ev := <-a:
		if ev != guardEventUninstall {
			t.Fatalf("agent-a got %q", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("agent-a was not woken")
	}
	select {
	case ev := <-b:
		t.Fatalf("agent-b must not be woken, got %q", ev)
	default:
	}
}

func TestGuardEventHubBroadcastAndUnsubscribe(t *testing.T) {
	hub := &guardEventHub{waiters: map[string]map[chan string]struct{}{}}
	a := hub.subscribe("agent-a")
	b := hub.subscribe("agent-b")
	hub.unsubscribe("agent-b", b)

	hub.broadcast(guardEventRebuild)
	hub.broadcast(guardEventRebuild) // buffered channel full: must not block

	if ev := <-a; ev != guardEventRebuild {
		t.Fatalf("agent-a got %q", ev)
	}
	select {
	case ev := <-b:
		t.Fatalf("unsubscribed agent-b got %q", ev)
	default:
	}
	hub.unsubscribe("agent-a", a)
	if len(hub.waiters) != 0 {
		t.Fatalf("waiters not cleaned up: %d", len(hub.waiters))
	}
}
