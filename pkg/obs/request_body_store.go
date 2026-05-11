package obs

import (
	"strconv"
	"sync"
)

// RequestBodyStore stores request bodies in memory for debugging. Storage
// is bounded by two limits which together form an envelope: a soft record
// count (to keep per-id overhead predictable) and a byte budget (to cap
// total memory). When either limit is exceeded the oldest entries are
// evicted in FIFO order.
//
// Bodies are stored intact — there is no per-entry truncation. A single
// very large request (e.g. a 1M-context LLM call) may consume most of the
// byte budget and push older entries out, but the body itself is retained
// in full so debugging is not handicapped by silent truncation.
type RequestBodyStore struct {
	bodies     map[string]*RequestBodyEntry
	order      []string // insertion order for FIFO eviction
	maxRecords int
	maxBytes   int64
	bytes      int64
	entrySeq   int64
	mu         sync.RWMutex
}

// RequestBodyEntry represents a stored request body with metadata.
type RequestBodyEntry struct {
	ID     string // Unique identifier (e.g., "req_1234567890")
	Method string // HTTP method
	Path   string // Request path
	Body   string // Request body (verbatim; never truncated by the store)
}

// NewRequestBodyStore creates a new request body store. maxRecords is a
// soft cap on the number of retained entries; maxBytes is the hard memory
// budget. Either may be zero to disable that particular limit (but at
// least one should be set to avoid unbounded growth).
func NewRequestBodyStore(maxRecords int, maxBytes int64) *RequestBodyStore {
	return &RequestBodyStore{
		bodies:     make(map[string]*RequestBodyEntry),
		maxRecords: maxRecords,
		maxBytes:   maxBytes,
	}
}

// ReserveID allocates a unique ID without storing anything yet. Callers
// pair this with a later Store(id, ...) once the body is available; this
// lets the ID be propagated (e.g. into gin.Context) before the body is
// fully read.
func (s *RequestBodyStore) ReserveID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entrySeq++
	return generateRequestID(s.entrySeq)
}

// Store records a request body under the given id. If id is empty a new
// one is allocated. Returns the id that was used.
func (s *RequestBodyStore) Store(id, method, path, body string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id == "" {
		s.entrySeq++
		id = generateRequestID(s.entrySeq)
	}

	entry := &RequestBodyEntry{
		ID:     id,
		Method: method,
		Path:   path,
		Body:   body,
	}

	if prev, ok := s.bodies[id]; ok {
		// Reserved earlier without a body: just replace.
		s.bytes -= int64(len(prev.Body))
	} else {
		s.order = append(s.order, id)
	}
	s.bodies[id] = entry
	s.bytes += int64(len(body))

	s.evictLocked()

	return id
}

// Get retrieves a request body by ID. Returns nil if not found.
func (s *RequestBodyStore) Get(id string) *RequestBodyEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bodies[id]
}

// Clear removes all entries from the store.
func (s *RequestBodyStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies = make(map[string]*RequestBodyEntry)
	s.order = s.order[:0]
	s.bytes = 0
}

// Size returns the current number of stored entries.
func (s *RequestBodyStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.bodies)
}

// Bytes returns the current total number of stored bytes.
func (s *RequestBodyStore) Bytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bytes
}

// evictLocked drops oldest entries until both limits are satisfied. The
// caller must hold s.mu. If a single entry exceeds the byte budget on its
// own, it is retained intact; it will be evicted naturally when newer
// entries arrive.
func (s *RequestBodyStore) evictLocked() {
	for len(s.order) > 0 {
		overCount := s.maxRecords > 0 && len(s.order) > s.maxRecords
		overBytes := s.maxBytes > 0 && s.bytes > s.maxBytes
		if !overCount && !overBytes {
			return
		}
		// If only one entry remains and it alone exceeds the byte budget,
		// keep it: evicting would drop the very thing we wanted to debug.
		if overBytes && !overCount && len(s.order) == 1 {
			return
		}
		oldest := s.order[0]
		s.order = s.order[1:]
		if entry, ok := s.bodies[oldest]; ok {
			s.bytes -= int64(len(entry.Body))
			delete(s.bodies, oldest)
		}
	}
}

// generateRequestID generates a unique request ID from a sequence number.
func generateRequestID(seq int64) string {
	return "req_" + strconv.FormatInt(seq, 10)
}
