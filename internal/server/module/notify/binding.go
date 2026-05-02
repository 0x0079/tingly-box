package notify

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// ScenarioBinding declares how a single bot serves Claude Code hooks for
// one named scenario. Bindings live inside each bot's settings row as a
// JSON-encoded list under the `scenarios` column (see db.Settings).
type ScenarioBinding struct {
	// Name matches the :scenario path parameter of /tingly/:scenario/notify
	// and the TINGLY_SCENARIO env var the hook script sends.
	Name string `json:"name"`
	// ChatID is the IM chat the bot pushes to / prompts on.
	ChatID string `json:"chat_id"`
	// Events optionally restricts which Claude hook events this binding
	// handles. Empty list = all events.
	Events []string `json:"events,omitempty"`
	// PermissionPolicy controls fallback behavior when the user does not
	// respond in time or the bot is unreachable.
	PermissionPolicy PermissionPolicy `json:"permission_policy,omitempty"`
	// Notification controls how interactive prompts are delivered (button
	// keyboard vs plain numbered text). Maps to imbot's interaction Mode.
	Notification NotificationConfig `json:"notification,omitempty"`
}

// PermissionPolicy specifies the fallback decision when an interactive
// hook cannot collect an answer in time.
type PermissionPolicy struct {
	// OnTimeout is one of "allow", "deny", "ask" (default "deny").
	OnTimeout string `json:"on_timeout,omitempty"`
	// OnDisconnect is one of "allow", "deny", "ask" (default "deny").
	OnDisconnect string `json:"on_disconnect,omitempty"`
	// TotalBudgetSeconds caps how long the script may keep polling
	// (default 300s).
	TotalBudgetSeconds int `json:"total_budget_seconds,omitempty"`
}

// NotificationConfig is reserved for per-binding presentation knobs.
// Currently only `mode` is honored (auto / interactive / text).
type NotificationConfig struct {
	Mode string `json:"mode,omitempty"`
}

// resolvedBinding bundles a binding with the bot identity needed to send.
type resolvedBinding struct {
	Binding  ScenarioBinding
	BotUUID  string
	Platform string
	BotName  string
}

// BindingStore is the subset of the imbot settings store the resolver
// needs. Defining it as an interface keeps the resolver testable without
// spinning up SQLite.
type BindingStore interface {
	ListEnabledSettings() ([]db.Settings, error)
}

// BindingResolver matches a (scenario, hook event) pair to a single bot
// binding by scanning enabled bot settings. The resolver is read-only and
// safe for concurrent use.
type BindingResolver struct {
	store BindingStore
}

// NewBindingResolver constructs a resolver backed by the given store.
func NewBindingResolver(store BindingStore) *BindingResolver {
	return &BindingResolver{store: store}
}

// Resolve returns the first enabled bot whose binding matches scenario +
// hookEvent. Returns nil + nil when no binding exists; returns an error
// only on store failures.
func (r *BindingResolver) Resolve(scenario, hookEvent string) (*resolvedBinding, error) {
	if r == nil || r.store == nil {
		return nil, nil
	}
	if scenario == "" {
		return nil, nil
	}
	settings, err := r.store.ListEnabledSettings()
	if err != nil {
		return nil, fmt.Errorf("list enabled settings: %w", err)
	}
	for _, s := range settings {
		bindings, err := parseBindings(s.Scenarios)
		if err != nil {
			// Don't let one malformed row block routing for the rest.
			continue
		}
		for _, b := range bindings {
			if b.Name != scenario {
				continue
			}
			if !eventAllowed(b.Events, hookEvent) {
				continue
			}
			return &resolvedBinding{
				Binding:  b,
				BotUUID:  s.UUID,
				Platform: s.Platform,
				BotName:  s.Name,
			}, nil
		}
	}
	return nil, nil
}

// parseBindings deserializes the JSON-encoded scenarios column. Empty
// input returns nil without error so unbound bots are a no-op.
func parseBindings(raw string) ([]ScenarioBinding, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []ScenarioBinding
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse scenarios: %w", err)
	}
	return out, nil
}

// eventAllowed reports whether the binding handles the given hook event.
// An empty Events list means "all events".
func eventAllowed(events []string, hookEvent string) bool {
	if len(events) == 0 {
		return true
	}
	for _, e := range events {
		if strings.EqualFold(e, hookEvent) {
			return true
		}
	}
	return false
}

// IsInteractiveEvent reports whether a hook event needs a user response
// fed back to Claude. Push-only events (Stop, PostToolUse) do not.
func IsInteractiveEvent(input ClaudeCodeHookInput) bool {
	switch input.HookEventName {
	case "PreToolUse":
		return true
	case "Notification":
		// Claude Code notifications without a permission context are
		// informational only. The matcher we install in apply_config.go
		// is "permission", so server-side we accept any Notification with
		// a body that suggests an approval ask.
		return strings.Contains(strings.ToLower(input.NotificationMessage), "permission") ||
			strings.Contains(strings.ToLower(input.NotificationMessage), "approve")
	}
	// PreToolUse with tool_name "AskUserQuestion" is the question flow;
	// it's covered by the PreToolUse branch above.
	return false
}
