package notify

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
)

// hookRequestID derives a stable request ID from the hook payload so
// Claude retries (which re-fire the same hook with the same session and
// tool_use_id) collapse into a single pending IM prompt instead of
// spamming the user.
func hookRequestID(input ClaudeCodeHookInput) string {
	parts := []string{
		input.SessionID,
		input.HookEventName,
		input.ToolName,
		input.ToolInput,
		input.NotificationMessage,
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// buildAskRequest converts a Claude hook payload + a resolved binding
// into the ask.Request the IMPrompter understands. ToolName is set to
// the actual Claude tool ("Bash", "Edit", "AskUserQuestion", ...) so the
// IMPrompter's existing tool-handler registry picks the right card.
func buildAskRequest(input ClaudeCodeHookInput, rb *resolvedBinding) (ask.Request, error) {
	if rb == nil {
		return ask.Request{}, fmt.Errorf("nil binding")
	}
	timeout := defaultBudget(rb.Binding.PermissionPolicy.TotalBudgetSeconds)
	req := ask.Request{
		ID:        hookRequestID(input),
		Type:      askType(input),
		ChatID:    rb.Binding.ChatID,
		Platform:  rb.Platform,
		BotUUID:   rb.BotUUID,
		SessionID: input.SessionID,
		AgentType: agentboot.AgentTypeClaude,
		ToolName:  toolNameForHook(input),
		Input:     toolInputForHook(input),
		Message:   buildPromptMessage(input),
		Reason:    fmt.Sprintf("Claude Code hook: %s", input.HookEventName),
		Timeout:   timeout,
	}
	return req, nil
}

func askType(input ClaudeCodeHookInput) ask.Type {
	if input.ToolName == "AskUserQuestion" {
		return ask.TypeQuestion
	}
	return ask.TypePermission
}

// toolNameForHook returns the tool the user is being asked about. For a
// bare permission Notification (no tool_name), we use a stand-in so the
// prompter still shows Approve/Deny buttons via its default builder.
func toolNameForHook(input ClaudeCodeHookInput) string {
	if input.ToolName != "" {
		return input.ToolName
	}
	return "ClaudeCode"
}

// toolInputForHook unmarshals the raw tool_input string Claude gives us
// into a map the IMPrompter's builders expect (specifically for
// AskUserQuestion which reads `questions` out of Input).
func toolInputForHook(input ClaudeCodeHookInput) map[string]interface{} {
	out := map[string]interface{}{}
	if strings.TrimSpace(input.ToolInput) != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(input.ToolInput), &parsed); err == nil {
			for k, v := range parsed {
				out[k] = v
			}
		} else {
			out["_raw_input"] = input.ToolInput
		}
	}
	if input.LastAssistantMessage != "" {
		out["_last_assistant_message"] = input.LastAssistantMessage
	}
	if input.NotificationMessage != "" {
		out["_notification_message"] = input.NotificationMessage
	}
	return out
}

// buildPromptMessage assembles a one-line headline shown above the
// keyboard. The IMPrompter's tool-handler builders may override the body,
// but they fall back to this when no specific builder exists.
func buildPromptMessage(input ClaudeCodeHookInput) string {
	switch input.HookEventName {
	case "PreToolUse":
		if input.ToolName == "AskUserQuestion" {
			if input.LastAssistantMessage != "" {
				return input.LastAssistantMessage
			}
			return "Claude is asking a question"
		}
		return fmt.Sprintf("Claude wants to run `%s`", input.ToolName)
	case "Notification":
		if input.NotificationMessage != "" {
			return input.NotificationMessage
		}
		return "Claude needs your attention"
	default:
		return input.HookEventName
	}
}

func defaultBudget(seconds int) time.Duration {
	if seconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}
