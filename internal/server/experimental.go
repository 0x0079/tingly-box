package server

import (
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ApplySmartCompact checks if smart_compact should be applied for a scenario
func (s *Server) ApplySmartCompact(scenario typ.RuleScenario) bool {
	return s.config.GetScenarioFlag(scenario, config.FeatureSmartCompact)

}

// ApplyRecording checks if recording should be applied for a scenario
func (s *Server) ApplyRecording(scenario typ.RuleScenario) bool {
	return s.config.IsScenarioRecordingEnabled(scenario)
}

// IsFusionEnabled reports whether the global fusion-provider experiment is on.
// Fusion mode lets a single Provider entry expose both OpenAI- and
// Anthropic-compatible base URLs; when this flag is off, fusion fields on
// Provider records are inert and dispatch falls back to the legacy
// APIBase/APIStyle pair.
func (s *Server) IsFusionEnabled() bool {
	return s.config.GetScenarioFlag(typ.ScenarioGlobal, config.FeatureFusionProvider)
}
