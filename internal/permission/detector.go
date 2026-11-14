// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package permission

import (
	"context"

	"github.com/openconductorhq/openconductor/internal/config"
)

// Detector combines L1 pattern matching and an optional L2 LLM classifier to
// produce a ParsedPermission from raw terminal output.
type Detector struct {
	classifier *Classifier // nil when no LLM is configured
}

// NewDetector creates a Detector. classifier may be nil; in that case only L1
// pattern matching is used and L2 escalation is skipped.
func NewDetector(classifier *Classifier) *Detector {
	return &Detector{classifier: classifier}
}

// Detect classifies a permission request from terminal output.
//
// L1 pattern matching runs first; on a miss it escalates to the L2 classifier.
//
// Returns nil when:
//   - No L1 pattern matched AND no classifier is configured.
//   - The L2 classifier returned a result below confidenceThreshold.
//
// A nil result means "cannot classify; notify the user".
func (d *Detector) Detect(ctx context.Context, sessionName string, agentType config.AgentType, lines []string) *ParsedPermission {
	if p := TryMatch(agentType, lines); p != nil {
		return p
	}
	// L1 missed — fall through to L2 if available.

	if d.classifier == nil {
		return nil
	}

	p, err := d.classifier.Classify(ctx, sessionName, agentType, lines)
	if err != nil || p == nil {
		return nil
	}

	// Reject low-confidence L2 results.
	if p.Confidence < confidenceThreshold {
		return nil
	}

	return p
}
