package analytics

type Store interface {
	Increment(eventType string)
	Totals() map[string]uint64
	TotalFor(eventType string) uint64
	Reset() // for testing purpose
}
