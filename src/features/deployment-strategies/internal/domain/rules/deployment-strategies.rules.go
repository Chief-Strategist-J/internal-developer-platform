/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES DOMAIN RULES (V.3 role 03, P0-02, P0-03, P0-04, Case 04)

1. Purpose:
   - Pure domain validation rules for progressive delivery.
   - Enforces Invariant P0-03: Traffic-before-pods safety checks.
   - Enforces Invariant P0-04: Metric analysis breach evaluation.
   - Enforces Case 04: Expand/contract schema migration pre-flight checks.
2. Invariants:
   - Pure deterministic functions; zero side effects, IO, or external state.
   - Zero inline comments below this header.
*/

package rules

import (
	"errors"
	"fmt"
	"strings"

	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
)

var (
	ErrUnreadyCanaryReplicas   = errors.New("invariant P0-03 violation: canary pods are not 100% ready")
	ErrMissingSidecarProxy     = errors.New("invariant P0-03 violation: envoy sidecar proxy not injected or unhealthy")
	ErrBreakingMigrationBlock = errors.New("case 04 violation: non-expandable database migration blocks canary rollout")
	ErrInsufficientSamples     = errors.New("insufficient telemetry samples for conclusive analysis")
)

func ValidateCanaryReadiness(report contracts.PodHealthReport) error {
	if report.ReplicasDesired <= 0 || report.ReplicasReady < report.ReplicasDesired {
		return fmt.Errorf("%w: ready=%d desired=%d", ErrUnreadyCanaryReplicas, report.ReplicasReady, report.ReplicasDesired)
	}
	if !report.SidecarInjected || !report.SidecarHealthy {
		return ErrMissingSidecarProxy
	}
	return nil
}

func ValidateExpandContractMigration(migrationMetadata string) error {
	normalized := strings.ToLower(migrationMetadata)
	if strings.Contains(normalized, "drop column") ||
		strings.Contains(normalized, "rename column") ||
		strings.Contains(normalized, "drop table") ||
		strings.Contains(normalized, "alter column type") {
		return fmt.Errorf("%w: metadata contains non-expandable operations: %s", ErrBreakingMigrationBlock, migrationMetadata)
	}
	return nil
}

func EvaluateMetricThreshold(samples []float64, threshold float64, minSamples int) (contracts.AnalysisVerdict, float64, error) {
	if len(samples) < minSamples {
		return contracts.AnalysisVerdictInconclusive, 0.0, ErrInsufficientSamples
	}
	sum := 0.0
	for _, val := range samples {
		sum += val
	}
	average := sum / float64(len(samples))
	if average > threshold {
		return contracts.AnalysisVerdictFail, average, nil
	}
	return contracts.AnalysisVerdictPass, average, nil
}
