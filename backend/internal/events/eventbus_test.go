package events

import (
	"context"
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

		err := eb.Publish(context.Background(), evt)
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
		err := eb.Publish(context.Background(), evt)
		if err != nil {
			t.Errorf("Publish should succeed with no handlers: %v", err)
		}
	})

	t.Run("publish with timeouts", func(t *testing.T) {
		eb := NewEventBus()

		handlerCalls := 0

		// Handler 1 : Fast
		fastHandler := func(evt Event) error {
			handlerCalls++
			time.Sleep(40 * time.Millisecond)
			return nil
		}

		// Handler 2 : Slow
		slowHandler := func(evt Event) error {
			handlerCalls++
			time.Sleep(100 * time.Millisecond)
			return nil
		}

		eb.Subscribe("TEST_EVENT", fastHandler)
		eb.Subscribe("TEST_EVENT", slowHandler)

		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
		defer cancel()

		evt := NewSystemEvent("TEST_EVENT", nil)
		err := eb.Publish(ctx, evt)

		if err != context.DeadlineExceeded {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}

		if handlerCalls != 1 {
			t.Errorf("Expected 1 handler call, got %d", handlerCalls)
		}
	})

	t.Run("publish stops on timeout", func(t *testing.T) {
		eb := NewEventBus()

		executed := []string{}

		for i := range 5 {
			id := fmt.Sprintf("handler-%d", i)
			eb.Subscribe("EVENT", func(evt Event) error {
				executed = append(executed, id)
				time.Sleep(20 * time.Millisecond)
				return nil
			})
		}

		// 5 handlers × 20ms = 100ms total
		// Timeout at 45ms = ~2 handlers
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Millisecond)
		defer cancel()

		eb.Publish(ctx, NewSystemEvent("EVENT", nil))

		// 2 handlers should have been exectuted
		if len(executed) >= 4 {
			t.Errorf("Too many handlers executed: %v", executed)
		}
	})
}

func TestEventBusErrorHandling(t *testing.T) {
	t.Run("handler returns error", func(t *testing.T) {
		eb := createTestEventBus(t)
		expectedErr := errors.New("handler error")
		handler := func(evt Event) error {
			return expectedErr
		}

		eb.Subscribe("ERRORED", handler)
		evt := NewSystemEvent("ERRORED", nil)
		err := eb.Publish(context.Background(), evt)
		if err == nil {
			t.Error("Publish should return handler error")
		}
		if err.Error() != "handler error" {
			t.Errorf("wrong error: got %v want %v", err, expectedErr)
		}
	})

	t.Run("multiple handlers stop on first error", func(t *testing.T) {
		eb := createTestEventBus(t)
		called := []string{}

		handlers := []struct {
			name string
			fn   EventHandler
		}{
			{"handler1", func(evt Event) error {
				called = append(called, "handler1")
				return nil
			}},
			{"handler2", func(evt Event) error {
				called = append(called, "handler2")
				return errors.New("boom")
			}},
			{"handler3", func(evt Event) error {
				called = append(called, "handler3")
				return nil
			}},
		}

		for _, h := range handlers {
			eb.Subscribe(TestEventType, h.fn)
		}

		evt := NewSystemEvent(TestEventType, nil)
		err := eb.Publish(context.Background(), evt)

		if err == nil {
			t.Fatal("expected error from handler2")
		}

		// Verify fail-fast behavior
		if len(called) != 2 {
			t.Errorf("expected 2 handlers called, got %d: %v", len(called), called)
		}
	})
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
				eb.Publish(context.Background(), NewSystemEvent(eventType, nil))
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
			eb.Publish(context.Background(), NewSystemEvent(fmt.Sprintf("EVENT_%d", i%5), nil))
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
		eb.Publish(context.Background(), evt)
	}
}
