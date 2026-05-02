// Package hookbridge wires Claude Code hooks to running IM bots.
//
// A running bot registers an Entry under its UUID. The notify HTTP module
// looks up the entry to either push a message (Stop / PostToolUse) or run
// an interactive prompt (PreToolUse / AskUserQuestion) whose result is
// long-polled back to the hook script.
package hookbridge

import (
	"context"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/ask"
)

// Prompter is the subset of the IM prompter that the bridge needs in order
// to drive interactive hook flows. The remote_control IMPrompter satisfies
// this interface naturally.
type Prompter interface {
	Prompt(ctx context.Context, req ask.Request) (ask.Result, error)
	SubmitResult(requestID string, result ask.Result) error
	GetPendingRequest(requestID string) (*ask.Request, bool)
}

// Sender is the subset of an imbot.Bot needed to push a one-shot text
// notification. Defining it here keeps the bridge package free of an
// imbot import.
type Sender interface {
	SendText(ctx context.Context, chatID, text string) error
}

// Entry captures the bot-level handles the bridge exposes to the notify
// module. Each running bot registers exactly one entry.
type Entry struct {
	BotUUID  string
	Platform string
	Prompter Prompter
	Sender   Sender
}

// Bridge holds per-bot entries plus a short-lived cache of recently
// answered interactive requests so a hook script reconnect that races
// with the user click still resolves to an answer rather than 404.
type Bridge struct {
	mu       sync.RWMutex
	entries  map[string]*Entry          // bot_uuid -> entry
	answers  map[string]*answeredEntry  // request_id -> result
	awaiters map[string]chan ask.Result // request_id -> waiter channel
	answerTL time.Duration
}

type answeredEntry struct {
	result    ask.Result
	expiresAt time.Time
}

// New creates an empty bridge. answerTTL controls how long completed
// results are retained for late wait reconnects (typical: 30s).
func New(answerTTL time.Duration) *Bridge {
	if answerTTL <= 0 {
		answerTTL = 30 * time.Second
	}
	return &Bridge{
		entries:  make(map[string]*Entry),
		answers:  make(map[string]*answeredEntry),
		awaiters: make(map[string]chan ask.Result),
		answerTL: answerTTL,
	}
}

// Register adds or replaces an entry for the given bot UUID.
func (b *Bridge) Register(entry *Entry) {
	if entry == nil || entry.BotUUID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[entry.BotUUID] = entry
}

// Unregister removes the entry for the given bot UUID.
func (b *Bridge) Unregister(botUUID string) {
	if botUUID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entries, botUUID)
}

// Get returns the entry for a bot UUID and whether it is registered.
func (b *Bridge) Get(botUUID string) (*Entry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	e, ok := b.entries[botUUID]
	return e, ok
}

// RememberAnswer caches a completed interactive result for late wait
// reconnects. Old entries are GC'd in the same call.
func (b *Bridge) RememberAnswer(requestID string, result ask.Result) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gcAnswersLocked()
	b.answers[requestID] = &answeredEntry{
		result:    result,
		expiresAt: time.Now().Add(b.answerTL),
	}
}

// LookupAnswer returns a previously remembered result if it has not yet
// expired.
func (b *Bridge) LookupAnswer(requestID string) (ask.Result, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gcAnswersLocked()
	e, ok := b.answers[requestID]
	if !ok {
		return ask.Result{}, false
	}
	return e.result, true
}

// AwaitChannel returns a channel that receives the result for a given
// request_id when it is delivered. The first caller for an ID owns the
// channel; subsequent calls receive the same channel. Callers must not
// close the channel; use SignalAnswer to deliver and clean up.
func (b *Bridge) AwaitChannel(requestID string) chan ask.Result {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.awaiters[requestID]; ok {
		return ch
	}
	ch := make(chan ask.Result, 1)
	b.awaiters[requestID] = ch
	return ch
}

// SignalAnswer fan-outs the result to the waiter channel (if any) and
// caches the answer for late reconnects. Safe to call from a goroutine
// running the underlying Prompter.
func (b *Bridge) SignalAnswer(requestID string, result ask.Result) {
	b.mu.Lock()
	ch, ok := b.awaiters[requestID]
	if ok {
		delete(b.awaiters, requestID)
	}
	b.gcAnswersLocked()
	b.answers[requestID] = &answeredEntry{
		result:    result,
		expiresAt: time.Now().Add(b.answerTL),
	}
	b.mu.Unlock()
	if ok {
		select {
		case ch <- result:
		default:
		}
	}
}

// DropWaiter removes a waiter channel without delivering a result. Used
// when a request is cancelled or replaced.
func (b *Bridge) DropWaiter(requestID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.awaiters, requestID)
}

func (b *Bridge) gcAnswersLocked() {
	now := time.Now()
	for id, entry := range b.answers {
		if now.After(entry.expiresAt) {
			delete(b.answers, id)
		}
	}
}
