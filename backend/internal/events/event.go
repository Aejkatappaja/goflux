// Package events
package events

import (
	"time"

	"github.com/aejkatappaja/goflux/internal/utils"
)

type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	UserID    string         `json:"user_id,omitempty"`
	Payload   map[string]any `json:"payload"`
	Version   string         `json:"version"`
}

const EventVersion string = "1.0.0"

// NewUserEvent creates an event triggered by a user action
func NewUserEvent(eventType string, userID string, payload map[string]any) Event {
	return Event{
		ID:        utils.GenerateUUID(),
		Type:      utils.NormalizeEventType(eventType),
		Timestamp: time.Now(),
		UserID:    userID,
		Payload:   payload,
		Version:   EventVersion,
	}
}

// NewSystemEvent creates an event triggered by the system
func NewSystemEvent(eventType string, payload map[string]any) Event {
	return Event{
		ID:        utils.GenerateUUID(),
		Type:      utils.NormalizeEventType(eventType),
		Timestamp: time.Now(),
		UserID:    "",
		Payload:   payload,
		Version:   EventVersion,
	}
}

// IsSystemEvent checks if the event was triggered by the system
func IsSystemEvent(e Event) bool {
	return e.UserID == ""
}
