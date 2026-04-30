package onboarding

import (
	"context"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/data"
)

// Extractor is the contract handlers depend on. v1 ships a pure rule-based
// implementation; future variants (LLM-assisted, model-based) can satisfy the
// same contract without changing the handler or wire format.
type Extractor interface {
	Extract(ctx context.Context, input string) (candidates []Candidate, warnings []string, err error)
}

// vendor → known API key prefixes. Embedded here (not in providers.json) so
// the onboarding hint table stays an internal concern of the extractor.
var keyPrefixVendorMap = map[string]string{
	"sk-ant-":  "anthropic",
	"sk-or-":   "openrouter",
	"sk-proj-": "openai",
	"gsk_":     "groq",
	"xai-":     "xai",
	"AIza":     "google",
	"ds-":      "deepseek",
}

// vendor signal from environment variable names.
var envVarVendorMap = map[string]string{
	"ANTHROPIC_API_KEY":    "anthropic",
	"ANTHROPIC_BASE_URL":   "anthropic",
	"ANTHROPIC_AUTH_TOKEN": "anthropic",
	"OPENAI_API_KEY":       "openai",
	"OPENAI_BASE_URL":      "openai",
	"OPENAI_API_BASE":      "openai",
	"GROQ_API_KEY":         "groq",
	"DEEPSEEK_API_KEY":     "deepseek",
	"DEEPSEEK_BASE_URL":    "deepseek",
	"XAI_API_KEY":          "xai",
	"GEMINI_API_KEY":       "google",
	"GOOGLE_API_KEY":       "google",
	"OPENROUTER_API_KEY":   "openrouter",
	"OPENROUTER_BASE_URL":  "openrouter",
	"MISTRAL_API_KEY":      "mistral",
	"PERPLEXITY_API_KEY":   "perplexity",
	"COHERE_API_KEY":       "cohere",
	"TOGETHER_API_KEY":     "together",
	"FIREWORKS_API_KEY":    "fireworks",
	"CEREBRAS_API_KEY":     "cerebras",
	"DASHSCOPE_API_KEY":    "alibaba",
	"MOONSHOT_API_KEY":     "moonshot",
	"KIMI_API_KEY":         "moonshot",
	"ZHIPU_API_KEY":        "zhipu",
	"ZAI_API_KEY":          "zhipu",
	"SILICONFLOW_API_KEY":  "siliconflow",
	"MODELSCOPE_API_KEY":   "modelscope",
	"NOVITA_API_KEY":       "novita",
	"DEEPINFRA_API_KEY":    "deepinfra",
	"HYPERBOLIC_API_KEY":   "hyperbolic",
	"NVIDIA_API_KEY":       "nvidia",
	"BAIDU_API_KEY":        "baidu",
	"TENCENT_API_KEY":      "tencent",
	"DOUBAO_API_KEY":       "doubao",
}

var (
	urlRegex      = regexp.MustCompile(`https?://[^\s'"\x60<>]+`)
	envVarRegex   = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{3,})\s*[:=]\s*['"]?([^\s'"\x60]+)`)
	bearerRegex   = regexp.MustCompile(`(?i)Bearer\s+([A-Za-z0-9_\-\.]{8,})`)
	xApiKeyRegex  = regexp.MustCompile(`(?i)x-api-key\s*[:=]\s*['"]?([A-Za-z0-9_\-\.]{8,})`)
	keyPrefixRe   = regexp.MustCompile(`\b(sk-ant-[A-Za-z0-9_\-]+|sk-or-[A-Za-z0-9_\-]+|sk-proj-[A-Za-z0-9_\-]+|sk-[A-Za-z0-9_\-]{16,}|gsk_[A-Za-z0-9_\-]+|xai-[A-Za-z0-9_\-]+|AIza[A-Za-z0-9_\-]{30,}|ds-[A-Za-z0-9_\-]{16,})\b`)
	jsonAPIKeyRe  = regexp.MustCompile(`(?i)"api[_-]?key"\s*:\s*"([^"]+)"`)
	jsonBaseURLRe = regexp.MustCompile(`(?i)"base[_-]?url"\s*:\s*"([^"]+)"`)
)

// templateProvider is the minimal subset of data.ProviderTemplate the
// extractor needs. Decoupling from the full struct makes the extractor easy
// to fake in tests without spinning up a TemplateManager.
type templateProvider struct {
	ID               string
	Name             string
	Alias            string
	Icon             string
	VendorFamily     string
	CanonicalDomain  string
	BaseURLOpenAI    string
	BaseURLAnthropic string
	AuthType         string
}

// RuleExtractor implements Extractor with pure regex + registry matching.
// No LLM, no network calls.
type RuleExtractor struct {
	templates []*templateProvider
}

// NewRuleExtractor builds an extractor from a TemplateManager. OAuth-only
// providers are excluded — onboarding's paste/browse flow targets API key
// providers; OAuth has its own dedicated flow.
func NewRuleExtractor(tm *data.TemplateManager) *RuleExtractor {
	var providers []*templateProvider
	if tm != nil {
		for _, t := range tm.GetAllTemplates() {
			if t == nil {
				continue
			}
			if t.AuthType == "oauth" {
				continue
			}
			providers = append(providers, &templateProvider{
				ID:               t.ID,
				Name:             t.Name,
				Alias:            t.Alias,
				Icon:             t.Icon,
				VendorFamily:     t.VendorFamily,
				CanonicalDomain:  t.CanonicalDomain,
				BaseURLOpenAI:    t.BaseURLOpenAI,
				BaseURLAnthropic: t.BaseURLAnthropic,
				AuthType:         t.AuthType,
			})
		}
	}
	return &RuleExtractor{templates: providers}
}

// candidateBuilder accumulates evidence for a single provider during
// extraction, then renders to Candidate at the end.
type candidateBuilder struct {
	tmpl         *templateProvider
	score        float64
	matchReasons []string
	baseURL      string
	apiStyle     string
	token        string
}

func (b *candidateBuilder) addReason(weight float64, reason string) {
	b.score += weight
	b.matchReasons = append(b.matchReasons, reason)
}

// Extract scans the input text for URLs, env vars, bearer tokens, x-api-key
// headers, JSON fields, and known key prefixes. It cross-references each
// signal against the provider registry and returns up to 3 candidates ranked
// by confidence. A non-nil warning is added when URL host disagrees with the
// vendor implied by the API key prefix.
func (e *RuleExtractor) Extract(_ context.Context, input string) ([]Candidate, []string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil, nil
	}

	urls := urlRegex.FindAllString(input, -1)
	envVars := envVarRegex.FindAllStringSubmatch(input, -1)
	bearers := bearerRegex.FindAllStringSubmatch(input, -1)
	xKeys := xApiKeyRegex.FindAllStringSubmatch(input, -1)
	keyMatches := keyPrefixRe.FindAllString(input, -1)
	jsonKeys := jsonAPIKeyRe.FindAllStringSubmatch(input, -1)
	jsonURLs := jsonBaseURLRe.FindAllStringSubmatch(input, -1)

	// Collect all candidate tokens we saw, in the order encountered.
	var tokens []string
	for _, m := range bearers {
		if len(m) >= 2 {
			tokens = append(tokens, m[1])
		}
	}
	for _, m := range xKeys {
		if len(m) >= 2 {
			tokens = append(tokens, m[1])
		}
	}
	for _, m := range jsonKeys {
		if len(m) >= 2 {
			tokens = append(tokens, m[1])
		}
	}
	tokens = append(tokens, keyMatches...)
	// env var values that look key-like (skip URL-valued env vars).
	for _, m := range envVars {
		if len(m) < 3 {
			continue
		}
		val := m[2]
		if strings.HasPrefix(strings.ToLower(val), "http://") || strings.HasPrefix(strings.ToLower(val), "https://") {
			continue
		}
		tokens = append(tokens, val)
	}
	tokens = dedupeStrings(tokens)

	// Determine vendor signals from key prefixes and env var names.
	var keyVendor string
	pickedToken := firstNonEmpty(keyMatches...)
	for prefix, vendor := range keyPrefixVendorMap {
		if pickedToken != "" && strings.HasPrefix(pickedToken, prefix) {
			keyVendor = vendor
			break
		}
	}
	if pickedToken == "" {
		// Fall back: pick the longest token-like string we have.
		pickedToken = longestString(tokens)
	}

	envVendor := ""
	for _, m := range envVars {
		if len(m) < 3 {
			continue
		}
		name := strings.ToUpper(m[1])
		if v, ok := envVarVendorMap[name]; ok {
			envVendor = v
			break
		}
	}

	// Compute base URLs from explicit signals.
	allURLs := append([]string{}, urls...)
	for _, m := range jsonURLs {
		if len(m) >= 2 {
			allURLs = append(allURLs, m[1])
		}
	}
	for _, m := range envVars {
		if len(m) < 3 {
			continue
		}
		val := m[2]
		if strings.HasPrefix(strings.ToLower(val), "http://") || strings.HasPrefix(strings.ToLower(val), "https://") {
			allURLs = append(allURLs, val)
		}
	}
	allURLs = dedupeStrings(allURLs)

	builders := make(map[string]*candidateBuilder)

	// Signal 1: URL host → canonical_domain match.
	urlVendors := make(map[string]bool)
	for _, raw := range allURLs {
		host := hostOf(raw)
		if host == "" {
			continue
		}
		for _, t := range e.templates {
			if t.CanonicalDomain == "" {
				continue
			}
			if !hostMatchesDomain(host, t.CanonicalDomain) {
				continue
			}
			b := getOrInit(builders, t)
			b.addReason(0.45, "domain:"+host)
			b.baseURL = canonicalBaseURL(raw, t)
			b.apiStyle = inferAPIStyle(raw, t, b.apiStyle)
			if t.VendorFamily != "" {
				urlVendors[t.VendorFamily] = true
			}
		}
	}

	// Signal 2: env var vendor.
	if envVendor != "" {
		for _, t := range e.templates {
			if t.VendorFamily == envVendor {
				b := getOrInit(builders, t)
				b.addReason(0.25, "env:"+envVendor)
			}
		}
	}

	// Signal 3: key prefix vendor.
	if keyVendor != "" {
		for _, t := range e.templates {
			if t.VendorFamily == keyVendor {
				b := getOrInit(builders, t)
				b.addReason(0.30, "key_prefix:"+keyVendor)
			}
		}
	}

	// Attach token and fill in defaults for builders.
	for _, b := range builders {
		if b.baseURL == "" {
			b.baseURL = preferredBaseURL(b.tmpl, b.apiStyle)
		}
		if b.apiStyle == "" {
			b.apiStyle = preferredAPIStyle(b.tmpl)
		}
		// Prefer the token whose prefix matches this provider's vendor.
		b.token = pickTokenForVendor(tokens, b.tmpl.VendorFamily)
		if b.token == "" {
			b.token = pickedToken
		}
	}

	var warnings []string
	if keyVendor != "" && len(urlVendors) > 0 && !urlVendors[keyVendor] {
		warnings = append(warnings, "URL and API key prefix indicate different vendors; please double-check before saving.")
	}

	// Render and rank.
	var out []Candidate
	for _, b := range builders {
		conf := b.score
		// Cap confidence to 0.5 if URL/key disagreement was flagged AND this
		// candidate is one of the disagreeing parties.
		if len(warnings) > 0 && b.tmpl.VendorFamily != "" {
			if (keyVendor != "" && b.tmpl.VendorFamily == keyVendor) || urlVendors[b.tmpl.VendorFamily] {
				if conf > 0.5 {
					conf = 0.5
				}
			}
		}
		if conf > 1 {
			conf = 1
		}
		out = append(out, Candidate{
			ProviderID:   b.tmpl.ID,
			Name:         displayName(b.tmpl),
			Icon:         b.tmpl.Icon,
			BaseURL:      b.baseURL,
			APIStyle:     b.apiStyle,
			Token:        b.token,
			Confidence:   roundTo(conf, 2),
			MatchReasons: b.matchReasons,
			Protocols:    protocolsFor(b.tmpl),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Confidence > out[j].Confidence
	})
	if len(out) > 3 {
		out = out[:3]
	}
	return out, warnings, nil
}

// --- helpers ---

func getOrInit(m map[string]*candidateBuilder, t *templateProvider) *candidateBuilder {
	if b, ok := m[t.ID]; ok {
		return b
	}
	b := &candidateBuilder{tmpl: t}
	m[t.ID] = b
	return b
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// hostMatchesDomain returns true when host equals domain or is a subdomain of
// it. Avoid plain substring matching to prevent foo-anthropic.com from
// matching anthropic.com.
func hostMatchesDomain(host, domain string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	domain = strings.ToLower(strings.TrimSpace(domain))
	if host == "" || domain == "" {
		return false
	}
	if host == domain {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}

// canonicalBaseURL returns the template-preferred base URL when the input
// URL matches the template's canonical domain. Otherwise it falls back to
// either base URL the template advertises.
func canonicalBaseURL(rawURL string, t *templateProvider) string {
	if t.BaseURLOpenAI != "" && strings.Contains(rawURL, hostOf(t.BaseURLOpenAI)) {
		return t.BaseURLOpenAI
	}
	if t.BaseURLAnthropic != "" && strings.Contains(rawURL, hostOf(t.BaseURLAnthropic)) {
		return t.BaseURLAnthropic
	}
	if t.BaseURLOpenAI != "" {
		return t.BaseURLOpenAI
	}
	return t.BaseURLAnthropic
}

func inferAPIStyle(rawURL string, t *templateProvider, prev string) string {
	low := strings.ToLower(rawURL)
	switch {
	case strings.Contains(low, "/v1/messages"), strings.Contains(low, "anthropic"):
		return "anthropic"
	case strings.Contains(low, "/chat/completions"), strings.Contains(low, "/responses"), strings.Contains(low, "/v1/models"):
		return "openai"
	}
	if prev != "" {
		return prev
	}
	return preferredAPIStyle(t)
}

func preferredAPIStyle(t *templateProvider) string {
	if t.BaseURLOpenAI != "" && t.BaseURLAnthropic == "" {
		return "openai"
	}
	if t.BaseURLAnthropic != "" && t.BaseURLOpenAI == "" {
		return "anthropic"
	}
	if t.BaseURLOpenAI != "" {
		return "openai"
	}
	if t.BaseURLAnthropic != "" {
		return "anthropic"
	}
	return ""
}

func preferredBaseURL(t *templateProvider, style string) string {
	switch style {
	case "anthropic":
		if t.BaseURLAnthropic != "" {
			return t.BaseURLAnthropic
		}
		return t.BaseURLOpenAI
	default:
		if t.BaseURLOpenAI != "" {
			return t.BaseURLOpenAI
		}
		return t.BaseURLAnthropic
	}
}

func protocolsFor(t *templateProvider) []string {
	var out []string
	if t.BaseURLOpenAI != "" {
		out = append(out, "openai")
	}
	if t.BaseURLAnthropic != "" {
		out = append(out, "anthropic")
	}
	return out
}

func displayName(t *templateProvider) string {
	if t.Alias != "" {
		return t.Alias
	}
	return t.Name
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func longestString(in []string) string {
	var best string
	for _, s := range in {
		if len(s) > len(best) {
			best = s
		}
	}
	return best
}

func firstNonEmpty(in ...string) string {
	for _, s := range in {
		if s != "" {
			return s
		}
	}
	return ""
}

// pickTokenForVendor returns the first token whose known prefix maps to the
// given vendor, or empty if no such token is present.
func pickTokenForVendor(tokens []string, vendor string) string {
	if vendor == "" {
		return ""
	}
	for _, tok := range tokens {
		for prefix, v := range keyPrefixVendorMap {
			if v == vendor && strings.HasPrefix(tok, prefix) {
				return tok
			}
		}
	}
	return ""
}

func roundTo(v float64, places int) float64 {
	mult := 1.0
	for i := 0; i < places; i++ {
		mult *= 10
	}
	return float64(int(v*mult+0.5)) / mult
}
