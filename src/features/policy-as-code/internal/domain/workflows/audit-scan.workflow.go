/*
ALGORITHM & ARCHITECTURE BLUEPRINT: CONTINUOUS AUDIT SCAN WORKFLOW (L4 DETECTION)

1. Architecture Role:
  - Step 05 Domain Workflow: Implements offline continuous inventory audit scanner (§1.2 L4, §3.8, §11 SLI-3/SLI-5).
  - Execution Sequence:
  - Builds O(M) exception lookup table keyed by (PolicyID + ":" + WorkloadID).
  - Iterates over provided workload manifests across cluster namespaces.
  - Evaluates active policies in AUDIT, WARN, or DENY lifecycle states.
  - Respects active valid exception grants unless policy is non-exceptable.
  - Yields AuditScanReport containing aggregated findings, scan metrics, and duration.

2. Invariant & Performance Adherence:
  - Rule 07 (Loops L2, L3, L9): Sequential iteration over workload collection with pre-allocated finding slices.
  - Rule 06 (Conditions C8, C11): Guard clauses for empty collections, boundary verification, parenthesized conditions.
  - Zero inline comments inside method bodies; pure deterministic domain workflow.
*/
package workflows

import (
	"fmt"
	"strings"
	"time"

	"internal-developer-platform/src/features/policy-as-code/internal/contracts"
	"internal-developer-platform/src/features/policy-as-code/internal/domain/rules"
)

type WorkloadManifest struct {
	Namespace    string `json:"namespace"`
	WorkloadID   string `json:"workloadId"`
	ResourceJSON string `json:"resourceJson"`
}

type AuditScanInput struct {
	TenantID   string                            `json:"tenantId"`
	Workloads  []WorkloadManifest                `json:"workloads"`
	Policies   []contracts.PolicyRecord          `json:"policies"`
	Exceptions []contracts.PolicyExceptionRecord `json:"exceptions"`
	Timestamp  time.Time                         `json:"timestamp"`
}

type AuditScanFinding struct {
	PolicyID      string                     `json:"policyId"`
	ExternalID    string                     `json:"externalId"`
	WorkloadID    string                     `json:"workloadId"`
	Namespace     string                     `json:"namespace"`
	Severity      contracts.PolicySeverity   `json:"severity"`
	Enforcement   contracts.EnforcementPoint `json:"enforcement"`
	Message       string                     `json:"message"`
	DetectionPair string                     `json:"detectionPair"`
}

type AuditScanReport struct {
	TotalWorkloadsScanned  int                `json:"totalWorkloadsScanned"`
	TotalPoliciesEvaluated int                `json:"totalPoliciesEvaluated"`
	Findings               []AuditScanFinding `json:"findings"`
	DurationMs             int64              `json:"durationMs"`
}

func ExecuteAuditScan(input AuditScanInput) AuditScanReport {
	start := time.Now()
	if input.Timestamp.IsZero() {
		input.Timestamp = time.Now().UTC()
	}

	report := AuditScanReport{
		TotalWorkloadsScanned:  len(input.Workloads),
		TotalPoliciesEvaluated: 0,
		Findings:               make([]AuditScanFinding, 0),
		DurationMs:             0,
	}

	activeExceptions := make(map[string]bool, len(input.Exceptions))
	for _, exc := range input.Exceptions {
		if (exc.Status == contracts.ExceptionStatusActive) && (exc.ExpiresAt.After(input.Timestamp)) {
			key := fmt.Sprintf("%s:%s", exc.PolicyID, exc.WorkloadID)
			activeExceptions[key] = true
		}
	}

	evaluablePolicies := make([]contracts.PolicyRecord, 0, len(input.Policies))
	for _, p := range input.Policies {
		if (p.LifecycleState == contracts.LifecycleAudit) ||
			(p.LifecycleState == contracts.LifecycleWarn) ||
			(p.LifecycleState == contracts.LifecycleDeny) {
			evaluablePolicies = append(evaluablePolicies, p)
		}
	}
	report.TotalPoliciesEvaluated = len(evaluablePolicies)

	for _, workload := range input.Workloads {
		trimmedWorkload := strings.TrimSpace(workload.WorkloadID)
		trimmedNamespace := strings.TrimSpace(workload.Namespace)
		if trimmedWorkload == "" || trimmedNamespace == "" || strings.TrimSpace(workload.ResourceJSON) == "" {
			continue
		}

		for _, policy := range evaluablePolicies {
			hasException := false
			if (policy.Exceptable) && (rules.IsRuleExceptable(policy.ExternalID)) {
				key := fmt.Sprintf("%s:%s", policy.ID, trimmedWorkload)
				if activeExceptions[key] {
					hasException = true
				}
			}

			if hasException {
				continue
			}

			if evaluateManifestViolation(policy.RuleExpression, workload.ResourceJSON) {
				report.Findings = append(report.Findings, AuditScanFinding{
					PolicyID:      policy.ID,
					ExternalID:    policy.ExternalID,
					WorkloadID:    trimmedWorkload,
					Namespace:     trimmedNamespace,
					Severity:      policy.Severity,
					Enforcement:   policy.EnforcementPoint,
					Message:       fmt.Sprintf("continuous audit scan detected %s violation: %s", policy.ExternalID, policy.Description),
					DetectionPair: policy.DetectionPair,
				})
			}
		}
	}

	report.DurationMs = time.Since(start).Milliseconds()
	return report
}
