package analytics

import (
	"context"
	"sync"
	"testing"

	"github.com/aejkatappaja/goflux/internal/events"
)

func TestService_CountsByType(t *testing.T) {
	// Arrange
	bus := events.NewEventBus()
	store := NewMemoryStore()
	svc := NewService(bus, store, []string{"view"})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Act
	for range 3 {
		if err := bus.Publish(context.Background(), events.NewSystemEvent("view", nil)); err != nil {
			t.Fatalf("publish failed: %v", err)
		}
	}

	// Assert
	if got, want := svc.TotalFor("view"), uint64(3); got != want {
		t.Errorf("TotalFor(view) = %d, want %d", got, want)
	}
	totals := svc.Totals()
	if got := totals["view"]; got != 3 {
		t.Errorf("Totals()[view] = %d, want %d", got, 3)
	}
}

func TestService_IgnoresNonTracked(t *testing.T) {
	bus := events.NewEventBus()
	store := NewMemoryStore()
	svc := NewService(bus, store, []string{"view"})
	_ = svc.Start(context.Background())

	_ = bus.Publish(context.Background(), events.NewSystemEvent("click", nil))

	if got := svc.TotalFor("click"); got != 0 {
		t.Errorf("TotalFor(click) = %d, want 0", got)
	}
}

func TestService_StartIdempotent(t *testing.T) {
	bus := events.NewEventBus()
	store := NewMemoryStore()
	svc := NewService(bus, store, []string{"view"})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = svc.Start(context.Background()) }()
	go func() { defer wg.Done(); _ = svc.Start(context.Background()) }()
	wg.Wait()

	for range 10 {
		_ = bus.Publish(context.Background(), events.NewSystemEvent("view", nil))
	}
	if got, want := svc.TotalFor("view"), uint64(10); got != want {
		t.Errorf("TotalFor(view) = %d, want %d", got, want)
	}
}

func TestService_ConcurrentPublishes(t *testing.T) {
	bus := events.NewEventBus()
	store := NewMemoryStore()
	svc := NewService(bus, store, []string{"view"})
	_ = svc.Start(context.Background())

	const G = 100
	const N = 10

	var wg sync.WaitGroup
	wg.Add(G)
	for range G {
		go func() {
			defer wg.Done()
			for range N {
				_ = bus.Publish(context.Background(), events.NewSystemEvent("view", nil))
			}
		}()
	}
	wg.Wait()

	if got, want := svc.TotalFor("view"), uint64(G*N); got != want {
		t.Errorf("TotalFor(view) = %d, want %d", got, want)
	}
}

func TestService_NormalizationOnRead(t *testing.T) {
	// Arrange
	bus := events.NewEventBus()
	store := NewMemoryStore()
	svc := NewService(bus, store, []string{"view"})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Act: publish with different casings/spacing
	cases := []string{"VIEW", " view ", "vIeW"}
	for _, typ := range cases {
		if err := bus.Publish(context.Background(), events.NewSystemEvent(typ, nil)); err != nil {
			t.Fatalf("publish failed: %v", err)
		}
	}

	// Assert
	if got, want := svc.TotalFor("view"), uint64(3); got != want {
		t.Errorf("TotalFor(view) = %d, want %d", got, want)
	}
	// Optionally verify read with mixed case still works
	if got, want := svc.TotalFor("ViEw"), uint64(3); got != want {
		t.Errorf("TotalFor(ViEw) = %d, want %d", got, want)
	}
}

func TestService_TotalsSnapshotIsImmutable(t *testing.T) {
	// Arrange
	bus := events.NewEventBus()
	store := NewMemoryStore()
	svc := NewService(bus, store, []string{"view"})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Seed one event
	if err := bus.Publish(context.Background(), events.NewSystemEvent("view", nil)); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	// Act: take a snapshot and mutate it locally
	totals := svc.Totals()
	if totals["view"] != 1 {
		t.Fatalf("precondition failed: Totals()[view] = %d, want 1", totals["view"])
	}
	totals["view"] = 999 // mutate local snapshot

	// Assert: internal store should be unchanged
	if got, want := svc.TotalFor("view"), uint64(1); got != want {
		t.Errorf("TotalFor(view) after snapshot mutation = %d, want %d", got, want)
	}

	// Re-read a fresh snapshot to ensure immutability
	totals2 := svc.Totals()
	if got, want := totals2["view"], uint64(1); got != want {
		t.Errorf("Totals()[view] after snapshot mutation = %d, want %d", got, want)
	}
}
