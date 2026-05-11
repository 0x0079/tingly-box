package obs

import (
	"sync"
	"time"
)

// StreamEventStore keeps raw streaming response events in memory for
// post-mortem debugging. Events are appended per-record (keyed by an ID
// shared with RequestBodyStore, typically the body_ref) and the whole
// store is bounded by a total byte budget — when exceeded, the oldest
// record is evicted in FIFO order. There is no per-event truncation: a
// single record may consume most of the budget if the response is large,
// but it will be retained intact so the original stream can be examined.
type StreamEventStore struct {
	records  map[string]*StreamRecord
	order    []string // insertion order for FIFO eviction
	maxBytes int64
	bytes    int64
	mu       sync.Mutex
}

// StreamRecord holds the ordered events captured for a single request.
type StreamRecord struct {
	ID        string        `json:"id"`
	StartTime time.Time     `json:"start_time"`
	Events    []StreamEvent `json:"events"`
	Closed    bool          `json:"closed"`
	bytes     int64         // cached size of all event Data + small overhead
}

// StreamEvent is one raw SSE/JSON event observed on the wire.
//
// Kind distinguishes provider-side (upstream raw bytes) from client-side
// (what tingly-box ultimately wrote back to the caller) so that protocol
// conversions can be inspected on both ends.
type StreamEvent struct {
	TS   time.Time `json:"ts"`
	Type string    `json:"type"`
	Kind string    `json:"kind"`
	Data []byte    `json:"data"`
}

// Kind values for StreamEvent.Kind.
const (
	StreamKindProvider = "provider" // upstream provider events
	StreamKindClient   = "client"   // what we sent to the client
)

// perEventOverhead approximates per-event slice header/struct overhead so
// the byte budget accounts for it (not just Data length).
const perEventOverhead int64 = 96

// NewStreamEventStore creates an empty store with the given byte budget.
// A non-positive budget disables storage (Append becomes a no-op).
func NewStreamEventStore(maxBytes int64) *StreamEventStore {
	return &StreamEventStore{
		records:  make(map[string]*StreamRecord),
		maxBytes: maxBytes,
	}
}

// Append records one event under the given id, creating the record on
// first use. Oldest records are evicted to stay within the byte budget.
func (s *StreamEventStore) Append(id, kind, eventType string, data []byte) {
	if s == nil || s.maxBytes <= 0 || id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[id]
	if !ok {
		rec = &StreamRecord{
			ID:        id,
			StartTime: time.Now(),
		}
		s.records[id] = rec
		s.order = append(s.order, id)
	}

	// Copy the slice so the caller can reuse its buffer.
	buf := make([]byte, len(data))
	copy(buf, data)
	rec.Events = append(rec.Events, StreamEvent{
		TS:   time.Now(),
		Type: eventType,
		Kind: kind,
		Data: buf,
	})
	size := int64(len(buf)) + perEventOverhead
	rec.bytes += size
	s.bytes += size

	s.evictLocked()
}

// Close marks a record as finished. It does not flush anything; it just
// records that no more events are expected so callers can render a final
// state in the UI.
func (s *StreamEventStore) Close(id string) {
	if s == nil || id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.records[id]; ok {
		rec.Closed = true
	}
}

// Get returns the record for an id, or nil if it has been evicted.
func (s *StreamEventStore) Get(id string) *StreamRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return nil
	}
	// Return a shallow copy so callers can read without holding the lock.
	out := *rec
	out.Events = append([]StreamEvent(nil), rec.Events...)
	return &out
}

// Stats returns lightweight stats about the store.
func (s *StreamEventStore) Stats() (records int, bytes int64, maxBytes int64) {
	if s == nil {
		return 0, 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records), s.bytes, s.maxBytes
}

// Clear removes all records.
func (s *StreamEventStore) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make(map[string]*StreamRecord)
	s.order = s.order[:0]
	s.bytes = 0
}

// evictLocked drops the oldest records until total bytes fits the budget.
// Caller must hold s.mu. If a single record exceeds the budget on its
// own we keep it (intact); it will be evicted naturally once a newer
// record arrives — that way the original stream is never silently lost.
func (s *StreamEventStore) evictLocked() {
	for s.bytes > s.maxBytes && len(s.order) > 1 {
		oldest := s.order[0]
		s.order = s.order[1:]
		if rec, ok := s.records[oldest]; ok {
			s.bytes -= rec.bytes
			delete(s.records, oldest)
		}
	}
}
