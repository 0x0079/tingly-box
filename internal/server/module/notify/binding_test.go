package notify

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/data/db"
)

type fakeStore struct {
	settings []db.Settings
	err      error
}

func (f *fakeStore) ListEnabledSettings() ([]db.Settings, error) {
	return f.settings, f.err
}

func TestBindingResolver_NoStore(t *testing.T) {
	r := NewBindingResolver(nil)
	got, err := r.Resolve("claude_code", "Stop")
	if err != nil || got != nil {
		t.Fatalf("expected nil/nil, got %v / %v", got, err)
	}
}

func TestBindingResolver_NoMatch(t *testing.T) {
	store := &fakeStore{settings: []db.Settings{
		{UUID: "b1", Platform: "telegram", Scenarios: `[{"name":"other","chat_id":"1"}]`},
	}}
	r := NewBindingResolver(store)
	got, err := r.Resolve("claude_code", "Stop")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil binding, got %+v", got)
	}
}

func TestBindingResolver_FirstMatchWins(t *testing.T) {
	store := &fakeStore{settings: []db.Settings{
		{UUID: "b1", Platform: "telegram", Scenarios: `[{"name":"claude_code","chat_id":"chatA"}]`},
		{UUID: "b2", Platform: "feishu", Scenarios: `[{"name":"claude_code","chat_id":"chatB"}]`},
	}}
	r := NewBindingResolver(store)
	got, err := r.Resolve("claude_code", "Stop")
	if err != nil || got == nil {
		t.Fatalf("expected hit, got %+v err=%v", got, err)
	}
	if got.BotUUID != "b1" || got.Binding.ChatID != "chatA" {
		t.Fatalf("wrong binding: %+v", got)
	}
}

func TestBindingResolver_EventFilter(t *testing.T) {
	store := &fakeStore{settings: []db.Settings{
		{UUID: "b1", Platform: "telegram", Scenarios: `[{"name":"claude_code","chat_id":"x","events":["PreToolUse"]}]`},
	}}
	r := NewBindingResolver(store)
	if got, _ := r.Resolve("claude_code", "Stop"); got != nil {
		t.Fatalf("Stop should not match events=[PreToolUse], got %+v", got)
	}
	if got, _ := r.Resolve("claude_code", "PreToolUse"); got == nil {
		t.Fatal("PreToolUse should match events=[PreToolUse]")
	}
}

func TestBindingResolver_MalformedRowSkipped(t *testing.T) {
	store := &fakeStore{settings: []db.Settings{
		{UUID: "b1", Platform: "telegram", Scenarios: `not-json`},
		{UUID: "b2", Platform: "feishu", Scenarios: `[{"name":"claude_code","chat_id":"ok"}]`},
	}}
	r := NewBindingResolver(store)
	got, _ := r.Resolve("claude_code", "Stop")
	if got == nil || got.BotUUID != "b2" {
		t.Fatalf("expected b2, got %+v", got)
	}
}

func TestIsInteractiveEvent(t *testing.T) {
	cases := []struct {
		name string
		in   ClaudeCodeHookInput
		want bool
	}{
		{"stop is push", ClaudeCodeHookInput{HookEventName: "Stop"}, false},
		{"posttooluse is push", ClaudeCodeHookInput{HookEventName: "PostToolUse"}, false},
		{"pretooluse is interactive", ClaudeCodeHookInput{HookEventName: "PreToolUse"}, true},
		{"notification with permission word is interactive",
			ClaudeCodeHookInput{HookEventName: "Notification", NotificationMessage: "Claude needs permission"}, true},
		{"plain notification is push",
			ClaudeCodeHookInput{HookEventName: "Notification", NotificationMessage: "fyi"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsInteractiveEvent(c.in); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}
