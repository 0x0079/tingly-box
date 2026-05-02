package hookbridge

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/ask"
)

func TestRegisterAndGet(t *testing.T) {
	b := New(0)
	if _, ok := b.Get("missing"); ok {
		t.Fatal("expected miss")
	}
	b.Register(&Entry{BotUUID: "u1", Platform: "telegram"})
	got, ok := b.Get("u1")
	if !ok || got.Platform != "telegram" {
		t.Fatalf("expected hit, got %+v ok=%v", got, ok)
	}
	b.Unregister("u1")
	if _, ok := b.Get("u1"); ok {
		t.Fatal("expected miss after unregister")
	}
}

func TestSignalAndAwait(t *testing.T) {
	b := New(time.Second)
	ch := b.AwaitChannel("req-1")
	go func() {
		b.SignalAnswer("req-1", ask.Result{ID: "req-1", Approved: true})
	}()
	select {
	case got := <-ch:
		if !got.Approved {
			t.Fatal("expected approved")
		}
	case <-time.After(time.Second):
		t.Fatal("await timed out")
	}
	if got, ok := b.LookupAnswer("req-1"); !ok || !got.Approved {
		t.Fatalf("answer not cached: %+v ok=%v", got, ok)
	}
}

func TestAnswerExpires(t *testing.T) {
	b := New(20 * time.Millisecond)
	b.SignalAnswer("req", ask.Result{ID: "req"})
	if _, ok := b.LookupAnswer("req"); !ok {
		t.Fatal("expected immediate hit")
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok := b.LookupAnswer("req"); ok {
		t.Fatal("expected expiry")
	}
}

func TestAwaitIsIdempotent(t *testing.T) {
	b := New(0)
	a := b.AwaitChannel("x")
	c := b.AwaitChannel("x")
	if a != c {
		t.Fatal("expected same channel for repeat awaits")
	}
}
