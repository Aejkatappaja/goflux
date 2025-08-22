// Package analytics
package analytics

import (
	"maps"
	"sync"
)

// MemoryStore is an in-memory, thread-safe Store implementation.
// It keeps counters in a map protected by an RWMutex.
// Event types are expected to be pre-normalized by the caller
// (trimmed + lowercased) for consistent keys.
type MemoryStore struct {
	mu     sync.RWMutex
	counts map[string]uint64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		counts: make(map[string]uint64),
	}
}

// Increment atomically increases the counter for the given normalized eventType.
// Acquires a write lock; safe for concurrent writers/readers.
func (m *MemoryStore) Increment(eventType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counts[eventType]++
}

// TotalFor returns the current counter for the given normalized eventType.
// Acquires a read lock; safe for concurrent access.
func (m *MemoryStore) TotalFor(eventType string) uint64 {
	m.mu.RLock()
	result := m.counts[eventType]
	defer m.mu.RUnlock()
	return result
}

// Totals returns a defensive copy of all counters at a point in time.
// The snapshot is created under a read lock and the internal map is never exposed.
func (m *MemoryStore) Totals() map[string]uint64 {
	snapshot := make(map[string]uint64, len(m.counts))
	m.mu.RLock()
	maps.Copy(snapshot, m.counts)
	m.mu.RUnlock()
	return snapshot
}

// Reset clears all counters. Intended for tests and ephemeral setups.
// Acquires a write lock.
func (m *MemoryStore) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counts = make(map[string]uint64)
}
