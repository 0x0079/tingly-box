//go:build tdd_harness

package smartrouting

// This file exists ONLY when the `tdd_harness` build tag is active. It declares
// the surface that the Phase A test harness compiles against. Phase B will:
//
//   - Land the real `OpProcessor`, `ProcessorContext`, `RegisterProcessor`,
//     `LookupProcessor`, `OpLatestUserProxyVision` in untagged production
//     files (e.g. `processor.go`, `op.go` extension).
//   - DELETE this file in the same commit that introduces those production
//     definitions.
//
// Default builds (`go build ./...` without `-tags=tdd_harness`) never see this
// file, so trunk stays green.

import (
	"context"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// OpProcessor is the placeholder shape Phase B will commit to. An op may carry
// processing behavior in addition to its match predicate; the routing stage
// triggers Process on every collected processor of a matched rule, then lets
// the pipeline continue (implicit bypass).
type OpProcessor interface {
	Process(pctx *ProcessorContext) error
}

// ProcessorContext is the placeholder request envelope handed to Process.
type ProcessorContext struct {
	Ctx       context.Context
	Request   any
	ReqCtx    *RequestContext
	RuleIndex int
	OpUUID    string
	Services  []*loadbalance.Service
}

var (
	processorRegistryMu sync.RWMutex
	processorRegistry   = make(map[string]OpProcessor) // key: pos|op
)

func processorKey(pos SmartOpPosition, op SmartOpOperation) string {
	return string(pos) + "|" + string(op)
}

// RegisterProcessor installs a processor for (pos, op). Phase B will likely
// reject duplicate registrations; the placeholder permits overwrite so tests
// can decide their own contract.
func RegisterProcessor(pos SmartOpPosition, op SmartOpOperation, p OpProcessor) {
	processorRegistryMu.Lock()
	defer processorRegistryMu.Unlock()
	processorRegistry[processorKey(pos, op)] = p
}

// LookupProcessor retrieves a processor for (pos, op) if registered.
func LookupProcessor(pos SmartOpPosition, op SmartOpOperation) (OpProcessor, bool) {
	processorRegistryMu.RLock()
	defer processorRegistryMu.RUnlock()
	p, ok := processorRegistry[processorKey(pos, op)]
	return p, ok
}

// UnregisterProcessor removes a registered processor; used by tests via
// t.Cleanup. Phase B may keep, drop, or rename this — placeholder only.
func UnregisterProcessor(pos SmartOpPosition, op SmartOpOperation) {
	processorRegistryMu.Lock()
	defer processorRegistryMu.Unlock()
	delete(processorRegistry, processorKey(pos, op))
}

// OpLatestUserProxyVision is the placeholder constant Phase B will move into
// `op.go` alongside the real Operations registry entry.
const OpLatestUserProxyVision SmartOpOperation = "proxy_vision"
