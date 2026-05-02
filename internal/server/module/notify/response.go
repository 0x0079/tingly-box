package notify

import (
	"github.com/tingly-dev/tingly-box/agentboot/ask"
)

// hookDecision is the JSON the hook script prints to stdout to drive
// Claude Code's hook output protocol. For PreToolUse Claude reads
// `hookSpecificOutput.permissionDecision`; for AskUserQuestion the
// answers map is consumed directly. We also include a `system_message`
// surface for transparency in the Claude transcript.
type hookDecision struct {
	HookSpecificOutput map[string]interface{} `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string                 `json:"systemMessage,omitempty"`
}

// encodeDecision converts the IMPrompter's ask.Result into the JSON
// shape Claude Code hooks expect on stdout. The resulting map can be
// marshaled directly by the wait endpoint.
func encodeDecision(input ClaudeCodeHookInput, result ask.Result) hookDecision {
	switch input.ToolName {
	case "AskUserQuestion":
		// Claude expects the question answers in
		// hookSpecificOutput.additionalContext (a stringified map). The
		// IMPrompter populates result.UpdatedInput["answers"]; surface
		// that as both additionalContext and system_message so that
		// Claude continues with the user's selections.
		answers := map[string]interface{}{}
		if result.UpdatedInput != nil {
			if a, ok := result.UpdatedInput["answers"].(map[string]interface{}); ok {
				answers = a
			}
		}
		return hookDecision{
			HookSpecificOutput: map[string]interface{}{
				"hookEventName": input.HookEventName,
				"answers":       answers,
			},
			SystemMessage: summarizeAnswers(answers),
		}
	default:
		decision := "deny"
		if result.Approved {
			decision = "allow"
		}
		reason := result.Reason
		if reason == "" {
			if result.Approved {
				reason = "Approved via IM"
			} else {
				reason = "Denied via IM"
			}
		}
		return hookDecision{
			HookSpecificOutput: map[string]interface{}{
				"hookEventName":           input.HookEventName,
				"permissionDecision":      decision,
				"permissionDecisionReason": reason,
			},
			SystemMessage: reason,
		}
	}
}

// fallbackDecision builds the hook output for a timeout / disconnect
// path. It encodes the configured policy into Claude's permission
// decision schema so the agent keeps running deterministically.
func fallbackDecision(input ClaudeCodeHookInput, policy string, reason string) hookDecision {
	if policy == "" {
		policy = "deny"
	}
	if reason == "" {
		reason = "no IM response within budget"
	}
	return hookDecision{
		HookSpecificOutput: map[string]interface{}{
			"hookEventName":           input.HookEventName,
			"permissionDecision":      policy,
			"permissionDecisionReason": reason,
		},
		SystemMessage: reason,
	}
}

func summarizeAnswers(answers map[string]interface{}) string {
	if len(answers) == 0 {
		return "User did not answer"
	}
	out := "User answered:"
	for q, a := range answers {
		out += "\n- " + q + ": "
		if s, ok := a.(string); ok {
			out += s
		}
	}
	return out
}
