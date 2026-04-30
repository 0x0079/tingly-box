package onboarding

// Candidate is a provider configuration candidate returned by the extractor.
// It is the wire format the frontend consumes to pre-fill the provider
// creation dialog.
type Candidate struct {
	ProviderID   string   `json:"provider_id"`
	Name         string   `json:"name"`
	Icon         string   `json:"icon,omitempty"`
	BaseURL      string   `json:"base_url,omitempty"`
	APIStyle     string   `json:"api_style,omitempty"`
	Token        string   `json:"token,omitempty"`
	Confidence   float64  `json:"confidence"`
	MatchReasons []string `json:"match_reasons,omitempty"`
	Protocols    []string `json:"protocols,omitempty"`
}

// ExtractRequest is the body for POST /api/v1/onboarding/extract.
type ExtractRequest struct {
	Input string `json:"input"`
}

// ExtractData is the inner payload returned by the extractor.
type ExtractData struct {
	Candidates []Candidate `json:"candidates"`
	Warnings   []string    `json:"warnings,omitempty"`
}

// ExtractResponse mirrors the rest of the v1 envelope shape used elsewhere in
// the API.
type ExtractResponse struct {
	Success bool         `json:"success"`
	Data    *ExtractData `json:"data,omitempty"`
	Error   *ErrorDetail `json:"error,omitempty"`
}

// ErrorDetail is a minimal error envelope. Kept local to the module so the
// onboarding API does not depend on internal server types.
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
}
