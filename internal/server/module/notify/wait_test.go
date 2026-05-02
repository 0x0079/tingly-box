package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/hookbridge"
)

// fakePrompter is a hookbridge.Prompter that blocks until SubmitResult
// is called for the matching request ID. Used to drive the wait
// endpoint without spinning up a real bot.
type fakePrompter struct {
	mu      sync.Mutex
	results map[string]chan ask.Result
}

func newFakePrompter() *fakePrompter {
	return &fakePrompter{results: make(map[string]chan ask.Result)}
}

func (p *fakePrompter) Prompt(ctx context.Context, req ask.Request) (ask.Result, error) {
	p.mu.Lock()
	ch, ok := p.results[req.ID]
	if !ok {
		ch = make(chan ask.Result, 1)
		p.results[req.ID] = ch
	}
	p.mu.Unlock()
	select {
	case r := <-ch:
		return r, nil
	case <-ctx.Done():
		return ask.Result{}, ctx.Err()
	}
}

func (p *fakePrompter) SubmitResult(requestID string, result ask.Result) error {
	p.mu.Lock()
	ch, ok := p.results[requestID]
	if !ok {
		ch = make(chan ask.Result, 1)
		p.results[requestID] = ch
	}
	p.mu.Unlock()
	ch <- result
	return nil
}

func (p *fakePrompter) GetPendingRequest(string) (*ask.Request, bool) { return nil, false }

func TestNotifyAndWait_PreToolUseAllow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeStore{settings: []db.Settings{{
		UUID:      "bot-1",
		Platform:  "telegram",
		Enabled:   true,
		Scenarios: `[{"name":"claude_code","chat_id":"chat-1","permission_policy":{"on_timeout":"deny","total_budget_seconds":120}}]`,
	}}}
	bridge := hookbridge.New(time.Second)
	prompter := newFakePrompter()
	bridge.Register(&hookbridge.Entry{BotUUID: "bot-1", Platform: "telegram", Prompter: prompter})

	h := NewHandlerWithBridge(NewBindingResolver(store), bridge)
	router := gin.New()
	RegisterRoutes(router, h)

	srv := httptest.NewServer(router)
	defer srv.Close()

	body := `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":"{\"command\":\"ls\"}"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/tingly/claude_code/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var initial map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&initial); err != nil {
		t.Fatal(err)
	}
	if initial["kind"] != "interactive" {
		t.Fatalf("expected kind=interactive, got %v", initial["kind"])
	}
	requestID, _ := initial["request_id"].(string)
	waitURL, _ := initial["wait_url"].(string)
	if requestID == "" || waitURL == "" {
		t.Fatalf("missing request_id/wait_url: %+v", initial)
	}

	// Simulate a long-poll arriving before the user clicks; it should
	// time out with 504 inside ~1 second.
	tooEarly, err := http.Get(srv.URL + waitURL + "?timeout=1s")
	if err != nil {
		t.Fatal(err)
	}
	tooEarly.Body.Close()
	if tooEarly.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", tooEarly.StatusCode)
	}

	// Now submit the user's allow decision and the next wait should
	// receive it immediately.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = prompter.SubmitResult(requestID, ask.Result{ID: requestID, Approved: true, Reason: "ok"})
	}()
	resp2, err := http.Get(srv.URL + waitURL + "?timeout=2s")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var final map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&final); err != nil {
		t.Fatal(err)
	}
	if final["status"] != "answered" {
		t.Fatalf("expected status=answered, got %v", final["status"])
	}
	dec, ok := final["decision"].(map[string]interface{})
	if !ok {
		t.Fatalf("decision missing or wrong type: %+v", final)
	}
	hso, _ := dec["hookSpecificOutput"].(map[string]interface{})
	if hso["permissionDecision"] != "allow" {
		t.Fatalf("expected allow, got %v", hso["permissionDecision"])
	}
}
