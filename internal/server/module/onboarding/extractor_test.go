package onboarding

import (
	"context"
	"strings"
	"testing"
)

// newTestExtractor builds a RuleExtractor with a small fixed registry covering
// the providers exercised by the test cases below. Keeping the fixture local
// avoids coupling the test suite to the live providers.json.
func newTestExtractor() *RuleExtractor {
	return &RuleExtractor{templates: []*templateProvider{
		{
			ID:              "openai-com",
			Name:            "OpenAI",
			Icon:            "openai",
			VendorFamily:    "openai",
			CanonicalDomain: "api.openai.com",
			BaseURLOpenAI:   "https://api.openai.com/v1",
		},
		{
			ID:               "anthropic-com",
			Name:             "Anthropic",
			Icon:             "anthropic",
			VendorFamily:     "anthropic",
			CanonicalDomain:  "api.anthropic.com",
			BaseURLAnthropic: "https://api.anthropic.com",
		},
		{
			ID:              "openrouter-ai",
			Name:            "OpenRouter",
			Icon:            "openrouter",
			VendorFamily:    "openrouter",
			CanonicalDomain: "openrouter.ai",
			BaseURLOpenAI:   "https://openrouter.ai/api/v1",
		},
		{
			ID:              "deepseek-com",
			Name:            "DeepSeek",
			Icon:            "deepseek",
			VendorFamily:    "deepseek",
			CanonicalDomain: "api.deepseek.com",
			BaseURLOpenAI:   "https://api.deepseek.com",
		},
		{
			ID:              "groq-com",
			Name:            "Groq",
			Icon:            "groq",
			VendorFamily:    "groq",
			CanonicalDomain: "api.groq.com",
			BaseURLOpenAI:   "https://api.groq.com/openai/v1",
		},
	}}
}

func mustTopCandidate(t *testing.T, cands []Candidate) Candidate {
	t.Helper()
	if len(cands) == 0 {
		t.Fatalf("expected at least one candidate, got 0")
	}
	return cands[0]
}

func TestExtractor_EnvFile(t *testing.T) {
	ext := newTestExtractor()
	input := `# .env
OPENAI_API_KEY=sk-proj-abcdef1234567890abcdef
OPENAI_BASE_URL=https://api.openai.com/v1
`
	cands, warnings, err := ext.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("did not expect warnings, got %v", warnings)
	}
	top := mustTopCandidate(t, cands)
	if top.ProviderID != "openai-com" {
		t.Fatalf("expected top=openai-com, got %s", top.ProviderID)
	}
	if top.APIStyle != "openai" {
		t.Fatalf("expected api_style=openai, got %s", top.APIStyle)
	}
	if !strings.HasPrefix(top.Token, "sk-proj-") {
		t.Fatalf("expected sk-proj token, got %q", top.Token)
	}
	if top.Confidence < 0.9 {
		t.Fatalf("expected high confidence (>=0.9), got %v", top.Confidence)
	}
}

func TestExtractor_AnthropicCurl(t *testing.T) {
	ext := newTestExtractor()
	input := `curl https://api.anthropic.com/v1/messages \
  -H "x-api-key: sk-ant-api03-XYZxyz0123456789ABCDEF" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"claude-3-5-sonnet-latest","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}'`
	cands, warnings, err := ext.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("did not expect warnings, got %v", warnings)
	}
	top := mustTopCandidate(t, cands)
	if top.ProviderID != "anthropic-com" {
		t.Fatalf("expected top=anthropic-com, got %s", top.ProviderID)
	}
	if top.APIStyle != "anthropic" {
		t.Fatalf("expected api_style=anthropic, got %s", top.APIStyle)
	}
	if !strings.HasPrefix(top.Token, "sk-ant-") {
		t.Fatalf("expected sk-ant token, got %q", top.Token)
	}
	if top.Confidence < 0.7 {
		t.Fatalf("expected confidence >=0.7, got %v", top.Confidence)
	}
}

func TestExtractor_OpenAIDocSnippet(t *testing.T) {
	ext := newTestExtractor()
	input := `from openai import OpenAI
client = OpenAI(api_key="sk-proj-abcdefghijklmnopqrstuvwx")
resp = client.chat.completions.create(model="gpt-4o", messages=[{"role":"user","content":"hi"}])
`
	cands, _, err := ext.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	top := mustTopCandidate(t, cands)
	if top.ProviderID != "openai-com" {
		t.Fatalf("expected top=openai-com, got %s", top.ProviderID)
	}
	if !strings.HasPrefix(top.Token, "sk-proj-") {
		t.Fatalf("expected sk-proj token, got %q", top.Token)
	}
}

func TestExtractor_OpenRouterDoc(t *testing.T) {
	ext := newTestExtractor()
	input := `curl https://openrouter.ai/api/v1/chat/completions \
  -H "Authorization: Bearer sk-or-v1-abcdef0123456789abcdef" \
  -H "Content-Type: application/json"`
	cands, warnings, err := ext.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("did not expect warnings, got %v", warnings)
	}
	top := mustTopCandidate(t, cands)
	if top.ProviderID != "openrouter-ai" {
		t.Fatalf("expected top=openrouter-ai, got %s", top.ProviderID)
	}
	if top.APIStyle != "openai" {
		t.Fatalf("expected openai-style for openrouter, got %s", top.APIStyle)
	}
	if !strings.HasPrefix(top.Token, "sk-or-") {
		t.Fatalf("expected sk-or token, got %q", top.Token)
	}
}

func TestExtractor_ConflictingURLAndKey(t *testing.T) {
	ext := newTestExtractor()
	// URL points to anthropic, but key prefix says openrouter — should warn.
	input := `https://api.anthropic.com/v1/messages
sk-or-v1-someopenrouterkey0123456789`
	_, warnings, err := ext.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected a warning for url/key disagreement, got none")
	}
}

func TestExtractor_BareKey(t *testing.T) {
	ext := newTestExtractor()
	input := `sk-ant-api03-justakeynothingelse123456`
	cands, _, err := ext.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	top := mustTopCandidate(t, cands)
	if top.ProviderID != "anthropic-com" {
		t.Fatalf("expected anthropic-com from bare key, got %s", top.ProviderID)
	}
	if top.BaseURL == "" {
		t.Fatalf("expected base URL filled in from template, got empty")
	}
	if top.Confidence > 0.5 {
		t.Fatalf("expected lower confidence with single signal, got %v", top.Confidence)
	}
}

func TestExtractor_BareURL(t *testing.T) {
	ext := newTestExtractor()
	input := `Try https://api.deepseek.com/v1/chat/completions in your client.`
	cands, _, err := ext.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	top := mustTopCandidate(t, cands)
	if top.ProviderID != "deepseek-com" {
		t.Fatalf("expected deepseek-com from URL, got %s", top.ProviderID)
	}
	if top.Token != "" {
		t.Fatalf("expected empty token (none in input), got %q", top.Token)
	}
}

func TestExtractor_EmptyInput(t *testing.T) {
	ext := newTestExtractor()
	cands, warnings, err := ext.Extract(context.Background(), "   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 0 || len(warnings) != 0 {
		t.Fatalf("expected empty result for whitespace input, got cands=%v warnings=%v", cands, warnings)
	}
}

func TestExtractor_LimitsToThreeCandidates(t *testing.T) {
	ext := newTestExtractor()
	// Multiple URLs, each matching a different vendor.
	input := `https://api.openai.com
https://api.anthropic.com
https://openrouter.ai
https://api.deepseek.com
https://api.groq.com`
	cands, _, err := ext.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) > 3 {
		t.Fatalf("expected at most 3 candidates, got %d", len(cands))
	}
}
