package notify

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/ask"
)

func TestHookRequestIDStable(t *testing.T) {
	in := ClaudeCodeHookInput{
		SessionID:     "s1",
		HookEventName: "PreToolUse",
		ToolName:      "Bash",
		ToolInput:     `{"command":"ls"}`,
	}
	a := hookRequestID(in)
	b := hookRequestID(in)
	if a != b || a == "" {
		t.Fatalf("expected stable non-empty id, got %q vs %q", a, b)
	}
	in2 := in
	in2.ToolInput = `{"command":"rm -rf /"}`
	if hookRequestID(in2) == a {
		t.Fatal("changing tool_input must change id")
	}
}

func TestBuildAskRequest_Permission(t *testing.T) {
	rb := &resolvedBinding{
		Binding:  ScenarioBinding{Name: "claude_code", ChatID: "c1"},
		BotUUID:  "b1",
		Platform: "telegram",
	}
	req, err := buildAskRequest(ClaudeCodeHookInput{
		SessionID:     "s1",
		HookEventName: "PreToolUse",
		ToolName:      "Bash",
		ToolInput:     `{"command":"ls"}`,
	}, rb)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if req.Type != ask.TypePermission {
		t.Fatalf("expected permission, got %s", req.Type)
	}
	if req.ChatID != "c1" || req.Platform != "telegram" || req.BotUUID != "b1" {
		t.Fatalf("routing fields wrong: %+v", req)
	}
	if req.ToolName != "Bash" {
		t.Fatalf("tool name lost: %s", req.ToolName)
	}
	if cmd, _ := req.Input["command"].(string); cmd != "ls" {
		t.Fatalf("input not parsed: %+v", req.Input)
	}
}

func TestBuildAskRequest_Question(t *testing.T) {
	rb := &resolvedBinding{
		Binding:  ScenarioBinding{Name: "cc", ChatID: "c"},
		BotUUID:  "b",
		Platform: "telegram",
	}
	req, err := buildAskRequest(ClaudeCodeHookInput{
		SessionID:     "s",
		HookEventName: "PreToolUse",
		ToolName:      "AskUserQuestion",
		ToolInput:     `{"questions":[{"question":"go?","options":[{"label":"yes"},{"label":"no"}]}]}`,
	}, rb)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if req.Type != ask.TypeQuestion {
		t.Fatalf("expected question, got %s", req.Type)
	}
	qs, ok := req.Input["questions"].([]interface{})
	if !ok || len(qs) != 1 {
		t.Fatalf("questions not preserved: %+v", req.Input)
	}
}

func TestEncodeDecision_Permission(t *testing.T) {
	in := ClaudeCodeHookInput{HookEventName: "PreToolUse", ToolName: "Bash"}
	d := encodeDecision(in, ask.Result{Approved: true, Reason: "ok"})
	got, _ := d.HookSpecificOutput["permissionDecision"].(string)
	if got != "allow" {
		t.Fatalf("expected allow, got %q", got)
	}
	dDeny := encodeDecision(in, ask.Result{Approved: false, Reason: "no"})
	if dDeny.HookSpecificOutput["permissionDecision"] != "deny" {
		t.Fatalf("expected deny, got %v", dDeny.HookSpecificOutput["permissionDecision"])
	}
}

func TestEncodeDecision_Question(t *testing.T) {
	in := ClaudeCodeHookInput{HookEventName: "PreToolUse", ToolName: "AskUserQuestion"}
	d := encodeDecision(in, ask.Result{
		Approved: true,
		UpdatedInput: map[string]interface{}{
			"answers": map[string]interface{}{"go?": "yes"},
		},
	})
	answers, ok := d.HookSpecificOutput["answers"].(map[string]interface{})
	if !ok || answers["go?"] != "yes" {
		t.Fatalf("answers not surfaced: %+v", d.HookSpecificOutput)
	}
}

func TestFallbackDecision(t *testing.T) {
	in := ClaudeCodeHookInput{HookEventName: "PreToolUse"}
	d := fallbackDecision(in, "", "")
	if d.HookSpecificOutput["permissionDecision"] != "deny" {
		t.Fatalf("default policy should be deny")
	}
	d = fallbackDecision(in, "allow", "policy")
	if d.HookSpecificOutput["permissionDecision"] != "allow" {
		t.Fatalf("explicit allow lost")
	}
}
