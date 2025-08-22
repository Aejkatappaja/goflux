package analytics

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aejkatappaja/goflux/internal/events"
	"github.com/aejkatappaja/goflux/internal/utils"
)

// Service subscribes to selected event types on the EventBus and maintains
// aggregated analytics (per-type counters) via a Store.
// Tracked types are normalized (trim + lowercase) at construction and treated
// as immutable in v1. Service is safe to Start concurrently (idempotent).
type Service struct {
	bus          *events.EventBus
	store        Store
	trackedTypes map[string]struct{}
	mu           sync.RWMutex
	started      atomic.Bool
}

const ErrPanic string = "bus || store cannot be nil"

// NewService constructs a Service that tracks the provided event types.
// It panics if bus or store are nil. Event types are normalized (trim + lowercase)
// and deduplicated. The set of tracked types is considered immutable in v1.
func NewService(bus *events.EventBus, store Store, types []string) *Service {
	if bus == nil || store == nil {
		panic(ErrPanic)
	}

	tracked := make(map[string]struct{}, len(types))

	for _, t := range types {
		key := utils.NormalizeEventType(t)
		if key == "" {
			continue
		}
		tracked[key] = struct{}{}
	}

	return &Service{
		bus:          bus,
		store:        store,
		trackedTypes: tracked,
	}
}

// Start subscribes the service to the EventBus for each tracked type.
// It is idempotent: the first call registers the handlers; subsequent calls
// are no-ops. The handler is intentionally minimal (normalize type, filter,
// increment store) and performs no I/O. ctx is reserved for future Stop/teardown.
func (s *Service) Start(ctx context.Context) error {
	if !s.started.CompareAndSwap(false, true) {
		return nil
	}

	handler := func(e events.Event) error {
		t := utils.NormalizeEventType(e.Type)
		if _, ok := s.trackedTypes[t]; !ok {
			return nil
		}
		s.store.Increment(t)
		return nil
	}
	for t := range s.trackedTypes {
		s.bus.Subscribe(t, handler)
	}
	return nil
}

// Totals returns a snapshot of all per-type counters by delegating to the Store.
// The returned map is a defensive copy and can be freely modified by callers.
func (s *Service) Totals() map[string]uint64 {
	// TODO: return s.store.Totals()
	return nil
}

// TotalFor returns the current counter for the given event type.
// The input is normalized before delegating to the Store to ensure
// case-insensitive lookups.
func (s *Service) TotalFor(eventType string) uint64 {
	// TODO: key := utils.NormalizeEventType(eventType); return s.store.TotalFor(key)
	return 0
}
