package notify

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/hookbridge"
	"github.com/tingly-dev/tingly-box/pkg/notify"
	systemnotify "github.com/tingly-dev/tingly-box/pkg/notify/provider/system"
)

// ClaudeCodeHookInput represents the JSON payload Claude Code sends to hooks via stdin
//
//	{
//	   "session_id": "9db738b8-4ee6-447a-9623-6fbf507e8d90",
//	   "transcript_path": ".claude/projects/-/9db738b8-4ee6-447a-9623-6fbf507e8d90.jsonl",
//	   "cwd": "tingly-box-branch",
//	   "permission_mode": "default",
//	   "hook_event_name": "Stop",
//	   "stop_hook_active": false,
//	   "last_assistant_message": "Hi! I see you're looking at the script files. Need help with something?"
//	}
type ClaudeCodeHookInput struct {
	SessionID            string `json:"session_id"`
	TranscriptPath       string `json:"transcript_path"`
	Cwd                  string `json:"cwd"`
	PermissionMode       string `json:"permission_mode"`
	HookEventName        string `json:"hook_event_name"` // "Stop", "Notification", "PostToolUse", etc.
	StopHookActive       bool   `json:"stop_hook_active"`
	LastAssistantMessage string `json:"last_assistant_message"` // the assistant's last message text
	ToolName             string `json:"tool_name"`              // for PostToolUse / PreToolUse
	ToolInput            string `json:"tool_input"`             // for PostToolUse / PreToolUse
	ToolOutput           string `json:"tool_output"`            // for PostToolUse
	NotificationMessage  string `json:"notification_message"`   // for Notification hook
}

// Handler handles notification HTTP requests from Claude Code hooks.
//
// When a binding resolver and IM bridge are wired in, interactive hook
// events (PreToolUse, AskUserQuestion, permission notifications) are
// routed to a registered IM bot and the user's response is fed back to
// Claude via long-poll. Push-only events (Stop, PostToolUse) and
// unbound scenarios still fall through to the desktop notifier so the
// existing behavior is preserved.
type Handler struct {
	notifier notify.Notifier
	resolver *BindingResolver
	bridge   *hookbridge.Bridge
	inflight *inflightStore
}

// NewHandler creates a notification handler with desktop notification
// only. For IM-aware hook routing use NewHandlerWithBridge.
func NewHandler() *Handler {
	mux := notify.NewMultiplexer()
	mux.AddProvider(systemnotify.New(systemnotify.Config{AppName: "Tingly Box"}))
	return &Handler{notifier: mux, inflight: newInflightStore()}
}

// NewHandlerWithBridge creates a handler that can route Claude Code
// hooks to IM bots through the supplied bridge and binding resolver.
// Either may be nil to disable that side of the routing.
func NewHandlerWithBridge(resolver *BindingResolver, bridge *hookbridge.Bridge) *Handler {
	mux := notify.NewMultiplexer()
	mux.AddProvider(systemnotify.New(systemnotify.Config{AppName: "Tingly Box"}))
	return &Handler{
		notifier: mux,
		resolver: resolver,
		bridge:   bridge,
		inflight: newInflightStore(),
	}
}

// Notify receives a Claude Code hook event.
// POST /tingly/:scenario/notify
func (h *Handler) Notify(c *gin.Context) {
	scenario := c.Param("scenario")

	var input ClaudeCodeHookInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Try IM routing first (if configured for this scenario + event).
	if h.tryRouteToIM(c, scenario, input) {
		return
	}

	// Push-only fallback: keep the legacy desktop notification flow so
	// stock setups (no scenario binding) still surface hook activity.
	title, message := buildMessage(input)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = h.notifier.Send(ctx, &notify.Notification{
			Title:   title,
			Message: message,
			Level:   notify.LevelInfo,
		})
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true, "kind": "push"})
}

// tryRouteToIM resolves the scenario binding and either pushes the
// event to the bound bot (non-interactive events) or starts an
// interactive prompt and returns the wait_url for the script to
// long-poll. Returns true if it took ownership of the response.
func (h *Handler) tryRouteToIM(c *gin.Context, scenario string, input ClaudeCodeHookInput) bool {
	if h.resolver == nil || h.bridge == nil {
		return false
	}
	rb, err := h.resolver.Resolve(scenario, input.HookEventName)
	if err != nil {
		logrus.WithError(err).WithField("scenario", scenario).Warn("scenario resolve failed")
		return false
	}
	if rb == nil {
		return false
	}
	entry, ok := h.bridge.Get(rb.BotUUID)
	if !ok || entry == nil {
		// Bot is bound but not running — let the script see "no
		// binding" so it falls through silently.
		return false
	}

	if !IsInteractiveEvent(input) && input.ToolName != "AskUserQuestion" {
		// Push notification path. Best-effort: deliver async and ack
		// immediately so the hook script never blocks on Stop /
		// PostToolUse / informational Notifications.
		text := buildPushText(input)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if entry.Sender == nil {
				return
			}
			if err := entry.Sender.SendText(ctx, rb.Binding.ChatID, text); err != nil {
				logrus.WithError(err).WithField("bot_uuid", rb.BotUUID).Warn("push to IM failed")
			}
		}()
		c.JSON(http.StatusOK, gin.H{"ok": true, "kind": "push"})
		return true
	}

	// Interactive path. Build the ask.Request and start the prompter
	// in the background; the script's subsequent GET to wait_url will
	// long-poll for the result.
	if entry.Prompter == nil {
		return false
	}
	req, err := buildAskRequest(input, rb)
	if err != nil {
		logrus.WithError(err).Warn("build ask request failed")
		return false
	}

	// Idempotency: a Claude retry of the same hook (same SessionID +
	// HookEvent + ToolInput) reuses the existing pending request
	// instead of double-sending. AwaitChannel is keyed by request ID
	// and is safe to call repeatedly.
	if _, exists := h.inflight.Get(req.ID); !exists {
		h.inflight.Put(req.ID, &inflightContext{
			Input:   input,
			Policy:  rb.Binding.PermissionPolicy,
			Created: time.Now(),
		})
		runPrompter(h.bridge, entry.Prompter, req)
	}

	expiresAt := time.Now().Add(defaultBudget(rb.Binding.PermissionPolicy.TotalBudgetSeconds))
	c.JSON(http.StatusAccepted, gin.H{
		"kind":       "interactive",
		"request_id": req.ID,
		"wait_url":   "/tingly/" + scenario + "/wait/" + req.ID,
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
	return true
}

// buildPushText is a one-line text message for non-interactive events
// pushed to IM. Uses the same heuristic as buildMessage but flattens
// title + message into a single line.
func buildPushText(input ClaudeCodeHookInput) string {
	title, msg := buildMessage(input)
	if msg == "" {
		return title
	}
	return title + "\n" + msg
}

// buildMessage maps Claude Code hook events to notification title/message.
// Title line: event + shortened cwd (last 2 segments).
// Body line: context from the event (assistant message, tool name, etc).
func buildMessage(input ClaudeCodeHookInput) (string, string) {
	cwd := shortenPath(input.Cwd, 2)

	switch input.HookEventName {
	case "Stop":
		title := "Claude Code · " + cwd
		msg := "Task completed"
		if input.LastAssistantMessage != "" {
			msg = truncate(input.LastAssistantMessage, 120)
		}
		return title, msg

	case "Notification":
		title := "Claude Code · " + cwd
		msg := "Needs attention"
		if input.NotificationMessage != "" {
			msg = input.NotificationMessage
		}
		return title, msg

	case "PreToolUse":
		title := "Claude Code · " + cwd
		msg := input.ToolName
		if msg == "" {
			msg = "Waiting for input"
		}
		return title, msg

	case "PostToolUse":
		title := "Claude Code · " + cwd
		msg := input.ToolName
		if msg == "" {
			msg = "Tool call finished"
		}
		return title, msg

	default:
		return "Claude Code · " + cwd, input.HookEventName
	}
}

// shortenPath keeps at most the last n segments of a path, e.g. shortenPath("/a/b/c/d", 2) → "c/d"
func shortenPath(p string, n int) string {
	p = strings.TrimRight(p, "/")
	segments := strings.Split(p, "/")
	if len(segments) <= n {
		return p
	}
	return strings.Join(segments[len(segments)-n:], "/")
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
