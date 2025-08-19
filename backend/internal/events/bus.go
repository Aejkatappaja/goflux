// Package events
package events

import (
	"errors"
)

type EventHandler func(evt Event) error

type EventBus struct {
	Handlers map[string][]EventHandler
}

func NewEventBus() *EventBus {
	handlers := make(map[string][]EventHandler)

	return &EventBus{handlers}
}

var (
	ErrHandlerNotFound error = errors.New("handler not found")
	ErrNoHandlers      error = errors.New("no handler registered for event type")
)

// Subscribe registers a handler for the specified event type.
// Multiple handlers can be registered for the same event type.
// Uses Go's append behavior where append(nil, item) creates a new slice.
func (eb *EventBus) Subscribe(eventType string, handler EventHandler) {
	eb.Handlers[eventType] = append(eb.Handlers[eventType], handler)
}

// Publish sends an event to all registered handlers for the event type.
// Returns ErrHandlerNotFound if no handlers are registered.
// Stops and returns error if any handler fails (fail-fast).
func (eb *EventBus) Publish(event Event) error {
	handlers, exists := eb.Handlers[event.Type]
	if !exists {
		return ErrHandlerNotFound
	}
	for _, handler := range handlers {
		err := handler(event)
		if err != nil {
			return ErrNoHandlers
		}
	}
	return nil
}
