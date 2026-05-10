//go:build tdd_harness

package processor

// This file exists ONLY when the `tdd_harness` build tag is active. It is the
// minimal surface Phase A test sketches compile against. Phase C will:
//
//   - Land the real `VisionProxyProcessor`, `visionClient`, and provider
//     resolver wiring in untagged production files (vision_proxy.go and
//     a small adapter to client.ClientPool).
//   - DELETE this file in the same commit.
//
// Default builds never see this file, so trunk stays green.

import (
	"context"

	"github.com/sirupsen/logrus"

	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// visionClient is the small interface VisionProxyProcessor depends on. The
// real adapter (Phase C) wraps client.ClientPool and dispatches to either
// Anthropic or OpenAI based on the chosen service's provider transport. Tests
// substitute a fake without touching the SDK pool.
//
// Returning ("", nil) means "no description available" → fail-strip path.
// Returning an error is also fail-strip.
type visionClient interface {
	Describe(ctx context.Context, mediaType, base64Data, remoteURL string) (string, error)
}

// providerResolver is the subset of routing.ProviderResolver the processor
// needs (look up a provider by UUID to decide transport). Defined locally so
// the placeholder does not import internal/server/routing — Phase C can
// either keep this local interface or alias the routing one.
type providerResolver interface {
	GetProviderByUUID(uuid string) (*typ.Provider, error)
}

// VisionProxyProcessor — placeholder shape. Process is a no-op so Phase A
// tests run (and visibly fail) under -tags=tdd_harness. Phase C replaces this
// file entirely with the real implementation.
type VisionProxyProcessor struct {
	Client   visionClient
	Resolver providerResolver
	Logger   *logrus.Logger
}

// Process is a Phase A no-op. Tests assert mutations that only happen once
// Phase C lands the real implementation; under tdd_harness they are red.
func (p *VisionProxyProcessor) Process(_ *smartrouting.ProcessorContext) error {
	return nil
}
