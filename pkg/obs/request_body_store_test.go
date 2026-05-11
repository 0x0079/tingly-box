package obs

import (
	"strings"
	"testing"
)

func TestRequestBodyStore_StoreAndGet(t *testing.T) {
	s := NewRequestBodyStore(10, 0)

	id := s.Store("", "POST", "/v1/messages", `{"a":1}`)
	if id == "" {
		t.Fatal("expected id")
	}
	got := s.Get(id)
	if got == nil {
		t.Fatal("expected entry")
	}
	if got.Body != `{"a":1}` {
		t.Fatalf("unexpected body: %s", got.Body)
	}
}

func TestRequestBodyStore_ReserveThenStore(t *testing.T) {
	s := NewRequestBodyStore(10, 0)

	id := s.ReserveID()
	// Body comes later (e.g. once handler finishes reading).
	s.Store(id, "POST", "/v1/messages", "later-body")

	got := s.Get(id)
	if got == nil || got.Body != "later-body" {
		t.Fatalf("expected reserved id to be filled, got %+v", got)
	}
	if s.Size() != 1 {
		t.Fatalf("expected size=1, got %d", s.Size())
	}
}

func TestRequestBodyStore_CountEviction(t *testing.T) {
	s := NewRequestBodyStore(2, 0)
	id1 := s.Store("", "POST", "/a", "1")
	id2 := s.Store("", "POST", "/b", "2")
	id3 := s.Store("", "POST", "/c", "3")

	if s.Get(id1) != nil {
		t.Fatal("expected id1 to be evicted")
	}
	if s.Get(id2) == nil || s.Get(id3) == nil {
		t.Fatal("expected id2 and id3 to remain")
	}
}

func TestRequestBodyStore_ByteEviction(t *testing.T) {
	s := NewRequestBodyStore(0, 10) // tiny budget
	id1 := s.Store("", "POST", "/a", strings.Repeat("x", 6))
	id2 := s.Store("", "POST", "/b", strings.Repeat("y", 6))

	if s.Get(id1) != nil {
		t.Fatal("expected id1 to be evicted under byte pressure")
	}
	if s.Get(id2) == nil {
		t.Fatal("expected id2 to remain")
	}
}

func TestRequestBodyStore_SingleLargeRecordRetained(t *testing.T) {
	s := NewRequestBodyStore(0, 100)
	id := s.Store("", "POST", "/big", strings.Repeat("z", 1000))
	// One record alone exceeds the budget; the store keeps it intact so
	// the original payload remains debuggable.
	if got := s.Get(id); got == nil || len(got.Body) != 1000 {
		t.Fatalf("expected intact retention, got %+v", got)
	}
}
