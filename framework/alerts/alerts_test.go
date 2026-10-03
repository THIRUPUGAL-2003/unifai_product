package alerts

import (
	"sync"
	"testing"
	"time"
)

func TestEmitDedupesWithinWindow(t *testing.T) {
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		got []Event
	)
	SetSink(func(e Event) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
		wg.Done()
	})
	t.Cleanup(func() {
		SetSink(nil)
		SetDedupeWindow(DefaultDedupeWindow)
		mu.Lock()
		lastSent = map[string]time.Time{}
		mu.Unlock()
	})
	SetDedupeWindow(time.Minute)

	base := time.Now().UTC()
	wg.Add(3)
	Emit(Event{Kind: KindBudgetExceeded, Title: "Budget exhausted", DedupeKey: "vk-1", Time: base})
	Emit(Event{Kind: KindBudgetExceeded, Title: "Budget exhausted", DedupeKey: "vk-1", Time: base.Add(30 * time.Second)})
	Emit(Event{Kind: KindBudgetExceeded, Title: "Budget exhausted", DedupeKey: "vk-2", Time: base.Add(30 * time.Second)})
	Emit(Event{Kind: KindBudgetExceeded, Title: "Budget exhausted", DedupeKey: "vk-1", Time: base.Add(2 * time.Minute)})

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for alert delivery")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("expected 3 delivered alerts (repeat inside window suppressed), got %d", len(got))
	}
	for _, e := range got {
		if e.Severity != SeverityWarning {
			t.Fatalf("expected default severity %q, got %q", SeverityWarning, e.Severity)
		}
	}
}

func TestEmitWithoutSinkIsNoop(t *testing.T) {
	SetSink(nil)
	Emit(Event{Kind: KindRateLimited, Title: "Rate limit reached"})
	mu.Lock()
	defer mu.Unlock()
	if _, ok := lastSent[KindRateLimited+"|Rate limit reached"]; ok {
		t.Fatal("an event without a sink must not consume the dedupe slot")
	}
}
