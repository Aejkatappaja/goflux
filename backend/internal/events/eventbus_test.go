package events

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	TestEventType  = "TEST_EVENT"
	BenchEventType = "BENCH_EVENT"
)

func createTestEventBus(t *testing.T) *EventBus {
	t.Helper()
	return NewEventBus()
}

func TestEventBus_Subscribe(t *testing.T) {
	t.Run("valid user event", func(t *testing.T) {
		const eventUserID = "0"
		eventPayload := map[string]any{"CANCEL_ACTION": true, "RETRIES": 5}

		eb := createTestEventBus(t)
		handlerCalled := false

		handler := func(e Event) error {
			handlerCalled = true

			if e.UserID != eventUserID {
				t.Errorf("wrong UserID: got %s, want %s", e.UserID, eventUserID)
			}
			if e.Type != TestEventType {
				t.Errorf("wrong event type: got %s, want %s", e.Type, TestEventType)
			}
			if e.Payload["CANCEL_ACTION"] != true {
				t.Error("payload CANCEL_ACTION should be true")
			}

			return nil
		}

		eb.Subscribe(TestEventType, handler)
		evt := NewUserEvent(TestEventType, eventUserID, eventPayload)

		err := eb.Publish(evt)
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
		if !handlerCalled {
			t.Error("handler was not called")
		}
	})

	t.Run("publish with no handlers", func(t *testing.T) {
		eb := createTestEventBus(t)
		evt := NewSystemEvent("UNHEARD_EVENT", nil)

		// No handlers = should not fail
		err := eb.Publish(evt)
		if err != nil {
			t.Errorf("Publish should succeed with no handlers: %v", err)
		}
	})
}

func TestEventBus_PublishWithError(t *testing.T) {
	eb := NewEventBus()
	expectedErr := errors.New("handler error")
	handler := func(evt Event) error {
		return expectedErr
	}

	eb.Subscribe("ERRORED", handler)
	evt := NewSystemEvent("ERRORED", nil)
	err := eb.Publish(evt)
	if err == nil {
		t.Error("Publish should return handler error")
	}
	if err.Error() != "handler error" {
		t.Errorf("wrong error: got %v", err)
	}
}

func TestEventBus_PublishMultipleHandlers_StopsOnError(t *testing.T) {
	eb := NewEventBus()

	called := []string{}

	handler1 := func(evt Event) error {
		called = append(called, "handler1")
		return nil
	}

	handler2 := func(evt Event) error {
		called = append(called, "handler2")
		return errors.New("boom")
	}

	handler3 := func(evt Event) error {
		called = append(called, "handler3")
		return nil
	}

	const eventType string = "EVENT_TEST"

	eb.Subscribe(eventType, handler1)
	eb.Subscribe(eventType, handler2)
	eb.Subscribe(eventType, handler3)

	evt := NewSystemEvent(eventType, nil)
	err := eb.Publish(evt)

	if err == nil {
		t.Error("expected error")
	}

	// Confirm execution stopped at handler2 (handler 3 should not be called)
	if len(called) != 2 {
		t.Errorf("expected 2 handlers called, got %d: %v", len(called), called)
	}
}

// go test -race -v -run Concurrent ./internal/events
func TestEventBus_ConcurrentAccess(t *testing.T) {
	eb := NewEventBus()

	counter := int32(0)

	counterValueExpected := int32(1000)

	handler := func(evt Event) error {
		atomic.AddInt32(&counter, 1)
		return nil
	}

	const eventType string = "CONCURRENT"

	eb.Subscribe(eventType, handler)

	var wg sync.WaitGroup

	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				eb.Publish(NewSystemEvent(eventType, nil))
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&counter) != counterValueExpected {
		t.Errorf("Expected %d events, got %d", counterValueExpected, counter)
	}
}

func TestEventBus_MixedConcurrentOps(t *testing.T) {
	eb := NewEventBus()

	var wg sync.WaitGroup

	// Publishers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 100 {
			eb.Publish(NewSystemEvent(fmt.Sprintf("EVENT_%d", i%5), nil))
		}
	}()

	// Subscribers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 20 {
			eb.Subscribe(fmt.Sprintf("EVENT_%d", i%5), func(e Event) error {
				return nil
			})
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}

// go test -bench=. -benchtime=10s ./internal/events
func BenchmarkEventBus_Publish(b *testing.B) {
	eb := NewEventBus()
	handler := func(evt Event) error {
		return nil
	}

	eb.Subscribe(BenchEventType, handler)

	for b.Loop() {
		evt := NewSystemEvent(BenchEventType, nil)
		eb.Publish(evt)
	}
}
