// Package events
package events

import (
	"context"
	"sync"
)

type EventHandler func(evt Event) error

type EventBus struct {
	mu       sync.RWMutex
	Handlers map[string][]EventHandler
}

func NewEventBus() *EventBus {
	handlers := make(map[string][]EventHandler)

	return &EventBus{Handlers: handlers}
}

// Subscribe registers a handler for the specified event type.
// Multiple handlers can be registered for the same event type.
// Uses Go's append behavior where append(nil, item) creates a new slice.
func (eb *EventBus) Subscribe(eventType string, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.Handlers[eventType] = append(eb.Handlers[eventType], handler)
}

// Publish sends an event to all registered handlers for the event type.
// Returns ErrNoHandlers if no handlers are registered.
// Stops and returns error if any handler fails (fail-fast).
func (eb *EventBus) Publish(ctx context.Context, event Event) error {
	eb.mu.RLock()
	handlers, exists := eb.Handlers[event.Type]
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
