package notify

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/internal/hookbridge"
)

// detachContext returns a context unbound from any HTTP request lifetime
// so the prompter goroutine outlives the POST that started it.
func detachContext() context.Context {
	return context.Background()
}

// inflightContext stores the per-request data the wait endpoint needs to
// reply to long-poll reconnects: the original hook input (for shaping
// the hook output JSON) and the binding's policy (for the timeout
// fallback path).
type inflightContext struct {
	Input   ClaudeCodeHookInput
	Policy  PermissionPolicy
	Created time.Time
}

// inflightStore holds the contexts of interactive requests that have
// been registered but not yet answered. Entries live until either an
// answer arrives (then moved to the bridge's recentlyAnswered map and
// dropped from here) or the total budget elapses.
type inflightStore struct {
	mu      sync.Mutex
	entries map[string]*inflightContext
}

func newInflightStore() *inflightStore {
	return &inflightStore{entries: make(map[string]*inflightContext)}
}

func (s *inflightStore) Put(id string, ctx *inflightContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[id] = ctx
}

func (s *inflightStore) Get(id string) (*inflightContext, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.entries[id]
	return c, ok
}

func (s *inflightStore) Drop(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
}

// Wait services GET /tingly/:scenario/wait/:request_id?timeout=45s.
// It blocks up to the requested timeout (capped) for an answer; on
// timeout it returns 504 so the script can reconnect; on total-budget
// expiry it returns 410 with the policy fallback decision.
func (h *Handler) Wait(c *gin.Context) {
	if h.bridge == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	requestID := c.Param("request_id")
	if requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing request_id"})
		return
	}

	timeout := parseTimeout(c.Query("timeout"))

	// Already-answered fast path: the user clicked between two reconnects.
	if result, ok := h.bridge.LookupAnswer(requestID); ok {
		h.respondAnswered(c, requestID, result)
		return
	}

	inflight, ok := h.inflight.Get(requestID)
	if !ok {
		// Not in inflight and not in cache → either expired or never
		// registered. The script falls through quietly on 404.
		c.JSON(http.StatusNotFound, gin.H{"status": "expired"})
		return
	}

	// Total-budget guard: independent of the per-poll timeout, refuse to
	// keep blocking once the user-configured total budget has elapsed.
	totalBudget := defaultBudget(inflight.Policy.TotalBudgetSeconds)
	if time.Since(inflight.Created) > totalBudget {
		policy := normalizePolicy(inflight.Policy.OnTimeout)
		decision := fallbackDecision(inflight.Input, policy, "no IM response within total budget")
		h.inflight.Drop(requestID)
		h.bridge.DropWaiter(requestID)
		c.JSON(http.StatusGone, gin.H{
			"status":   "timeout",
			"fallback": policy,
			"decision": decision,
		})
		return
	}

	ch := h.bridge.AwaitChannel(requestID)
	select {
	case result := <-ch:
		h.respondAnswered(c, requestID, result)
	case <-time.After(timeout):
		c.JSON(http.StatusGatewayTimeout, gin.H{"status": "pending"})
	case <-c.Request.Context().Done():
		c.JSON(http.StatusGatewayTimeout, gin.H{"status": "pending"})
	}
}

func (h *Handler) respondAnswered(c *gin.Context, requestID string, result ask.Result) {
	inflight, _ := h.inflight.Get(requestID)
	var input ClaudeCodeHookInput
	if inflight != nil {
		input = inflight.Input
	}
	decision := encodeDecision(input, result)
	if isCancelResult(result) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "cancelled",
			"decision": decision,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status":   "answered",
			"decision": decision,
		})
	}
	h.inflight.Drop(requestID)
}

func isCancelResult(r ask.Result) bool {
	if r.Approved {
		return false
	}
	reason := r.Reason
	return reason == "cancel" || reason == "cancelled"
}

func parseTimeout(raw string) time.Duration {
	const (
		defaultTimeout = 45 * time.Second
		maxTimeout     = 50 * time.Second
	)
	if raw == "" {
		return defaultTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultTimeout
	}
	if d > maxTimeout {
		return maxTimeout
	}
	if d < time.Second {
		return time.Second
	}
	return d
}

// runPrompter calls the IM prompter in a goroutine and feeds the
// outcome to the bridge's awaiter. Errors are encoded as a denied
// result so the wait endpoint always sees a deterministic answer.
func runPrompter(bridge *hookbridge.Bridge, prompter hookbridge.Prompter, req ask.Request) {
	go func() {
		result, err := prompter.Prompt(detachContext(), req)
		if err != nil {
			result = ask.Result{
				ID:       req.ID,
				Approved: false,
				Reason:   "prompter error: " + err.Error(),
			}
		}
		bridge.SignalAnswer(req.ID, result)
	}()
}

// normalizePolicy maps the configured policy string into Claude's
// permissionDecision values; unknown / empty values default to "deny".
func normalizePolicy(policy string) string {
	switch policy {
	case "allow", "deny", "ask":
		return policy
	default:
		return "deny"
	}
}
