package obs

import (
	"strings"
	"testing"
)

func TestStreamEventStore_AppendAndGet(t *testing.T) {
	s := NewStreamEventStore(1024 * 1024)

	s.Append("req_1", StreamKindClient, "message_start", []byte(`{"type":"message_start"}`))
	s.Append("req_1", StreamKindClient, "content_block_delta", []byte(`{"type":"content_block_delta"}`))
	s.Close("req_1")

	rec := s.Get("req_1")
	if rec == nil {
		t.Fatal("expected record")
	}
	if len(rec.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(rec.Events))
	}
	if !rec.Closed {
		t.Fatal("expected Closed=true")
	}
	if rec.Events[0].Type != "message_start" {
		t.Fatalf("unexpected event[0].Type: %s", rec.Events[0].Type)
	}
}

func TestStreamEventStore_FIFOEviction(t *testing.T) {
	// Budget large enough for ~2 records of 1KB events; 3rd evicts the 1st.
	s := NewStreamEventStore(3000)

	for i, id := range []string{"a", "b", "c"} {
		_ = i
		s.Append(id, StreamKindClient, "ev", []byte(strings.Repeat("x", 1000)))
	}

	if s.Get("a") != nil {
		t.Fatal("expected 'a' to be evicted")
	}
	if s.Get("b") == nil || s.Get("c") == nil {
		t.Fatal("expected 'b' and 'c' to remain")
	}
}

func TestStreamEventStore_LargeRecordRetainedIntact(t *testing.T) {
	s := NewStreamEventStore(1024) // 1KB budget

	// A single 5KB event blows past the budget; it must still be retained
	// intact (no truncation). It will be evicted when a newer record
	// arrives, but for now it's the only thing in the store.
	big := []byte(strings.Repeat("y", 5000))
	s.Append("solo", StreamKindClient, "ev", big)

	rec := s.Get("solo")
	if rec == nil {
		t.Fatal("expected record to be retained despite exceeding budget")
	}
	if len(rec.Events) != 1 || len(rec.Events[0].Data) != 5000 {
		t.Fatalf("expected 1 event of 5000 bytes, got len=%d size=%d",
			len(rec.Events), len(rec.Events[0].Data))
	}
}

func TestStreamEventStore_DisabledByZeroBudget(t *testing.T) {
	s := NewStreamEventStore(0)
	s.Append("x", StreamKindClient, "ev", []byte("hi"))
	if s.Get("x") != nil {
		t.Fatal("expected no record when budget is zero")
	}
}
