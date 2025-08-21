// Package events
package events

import (
	"context"
	"sync"

	"github.com/aejkatappaja/goflux/internal/utils"
)

type EventHandler func(evt Event) error

type EventBus struct {
	mu       sync.RWMutex
	handlers map[string][]EventHandler
}

func NewEventBus() *EventBus {
	handlers := make(map[string][]EventHandler)

	return &EventBus{handlers: handlers}
}

// Subscribe registers a handler for the specified event type.
// Multiple handlers can be registered for the same event type.
// Uses Go's append behavior where append(nil, item) creates a new slice.
func (eb *EventBus) Subscribe(eventType string, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	key := utils.NormalizeEventType(eventType)
	eb.handlers[key] = append(eb.handlers[key], handler)
}

// Publish sends an event to all registered handlers for its (normalized) type.
// If no handlers are registered, it returns nil.
// It stops and returns the first handler error (fail-fast), and honors ctx
// cancellation/timeout between handlers and at the end.
func (eb *EventBus) Publish(ctx context.Context, event Event) error {
	// Lookup without mutating the event
	key := utils.NormalizeEventType(event.Type)

	eb.mu.RLock()
	handlers, exists := eb.handlers[key]
	eb.mu.RUnlock()

	if !exists || len(handlers) == 0 {
		return nil
	}

	for _, handler := range handlers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := handler(event); err != nil {
			return err
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}
