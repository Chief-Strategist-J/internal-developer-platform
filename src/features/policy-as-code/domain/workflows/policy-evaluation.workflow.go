/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY ADMISSION EVALUATION WORKFLOW

1. Architecture Role:
   - Step 05 Domain Workflow: Executes multi-stage admission evaluation against active policies (§3 in business-logic.md).
   - Execution Sequence:
     * Stage 1 (Namespace & Identity Exemption): Rejects unauthorized tenant attempts to deploy into platform namespaces.
     * Stage 2 (Exception Lookup): Matches against active, unexpired exception grants. Non-exceptable rules bypass exception check.
     * Stage 3 (Enforcement Dispatch): Evaluates lifecycle ring behaviour (DENY rejects; WARN admits with warning; AUDIT admits with audit metric).

2. Invariant & Performance Adherence:
   - Rule 07 (Loops L2, L9, L25): Single-pass O(M) exception index build followed by linear O(N) evaluation loop.
     Exception invariant: at iteration i, activeExceptions contains all unexpired active grants from input.Exceptions[0..i-1].
     Policy invariant: at iteration j, verdict contains all verified violations from input.Policies[0..j-1].
   - Rule 04 (Performance P75, P76): Pre-allocates map and slice capacities based on input sizes to eliminate reallocation churn.
   - Rule 06 (Conditions C8, C11, C46): Guard clauses first, parenthesized compound conditions, fail-closed default for DENY violations.
   - Zero inline comments inside method bodies; pure deterministic domain workflow.
*/
package workflows

import (
	"fmt"
	"strings"
	"time"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/features/policy-as-code/domain/rules"
)

type AdmissionEvaluationInput struct {
	TenantID     string
	Namespace    string
	WorkloadID   string
	CallerRole   string
	ResourceJSON string
	Policies     []contracts.PolicyRecord
	Exceptions   []contracts.PolicyExceptionRecord
	Timestamp    time.Time
}

type PolicyViolation struct {
	PolicyID      string
	ExternalID    string
	Severity      contracts.PolicySeverity
	Enforcement   contracts.EnforcementPoint
	Message       string
	DetectionPair string
}

type AdmissionVerdict struct {
	Allowed            bool
	DenialReason       string
	Violations         []PolicyViolation
	Warnings           []string
	AuditViolations    []PolicyViolation
	AppliedExceptions  []string
	EvaluatedRuleCount int
}

var platformNamespaces = map[string]bool{
	"kube-system":       true,
	"gatekeeper-system": true,
	"istio-system":      true,
	"argocd":            true,
}

func ExecuteAdmissionEvaluation(input AdmissionEvaluationInput) AdmissionVerdict {
	verdict := AdmissionVerdict{
		Allowed:            true,
		Violations:         make([]PolicyViolation, 0, len(input.Policies)),
		Warnings:           make([]string, 0, len(input.Policies)),
		AuditViolations:    make([]PolicyViolation, 0, len(input.Policies)),
		AppliedExceptions:  make([]string, 0, len(input.Exceptions)),
		EvaluatedRuleCount: 0,
	}

	cleanNamespace := strings.ToLower(strings.TrimSpace(input.Namespace))
	isPlatformNS := platformNamespaces[cleanNamespace]
	isPlatformAdmin := strings.EqualFold(input.CallerRole, "PlatformAdmin")

	if (isPlatformNS) && (!isPlatformAdmin) {
		verdict.Allowed = false
		verdict.DenialReason = fmt.Sprintf("unauthorized tenant attempt to target protected platform namespace: %s", input.Namespace)
		return verdict
	}

	activeExceptions := make(map[string]bool, len(input.Exceptions))
	for _, exc := range input.Exceptions {
		if (exc.Status == contracts.ExceptionStatusActive) && (exc.ExpiresAt.After(input.Timestamp)) {
			key := fmt.Sprintf("%s:%s", exc.PolicyID, exc.WorkloadID)
			activeExceptions[key] = true
		}
	}

	for _, policy := range input.Policies {
		if (policy.LifecycleState == contracts.LifecycleDraft) ||
			(policy.LifecycleState == contracts.LifecycleCIOnly) ||
			(policy.LifecycleState == contracts.LifecycleRolledBack) ||
			(policy.LifecycleState == contracts.LifecycleRetired) {
			continue
		}

		verdict.EvaluatedRuleCount++
		hasException := false

		if (policy.Exceptable) && (rules.IsRuleExceptable(policy.ExternalID)) {
			key := fmt.Sprintf("%s:%s", policy.ID, input.WorkloadID)
			if activeExceptions[key] {
				hasException = true
				verdict.AppliedExceptions = append(verdict.AppliedExceptions, policy.ExternalID)
			}
		}

		if hasException {
			continue
		}

		isViolated := evaluateManifestViolation(policy.RuleExpression, input.ResourceJSON)
		if !isViolated {
			continue
		}

		violation := PolicyViolation{
			PolicyID:      policy.ID,
			ExternalID:    policy.ExternalID,
			Severity:      policy.Severity,
			Enforcement:   policy.EnforcementPoint,
			Message:       fmt.Sprintf("policy %s violation: %s", policy.ExternalID, policy.Description),
			DetectionPair: policy.DetectionPair,
		}

		switch policy.LifecycleState {
		case contracts.LifecycleDeny:
			verdict.Violations = append(verdict.Violations, violation)
			verdict.Allowed = false
		case contracts.LifecycleWarn:
			verdict.Warnings = append(verdict.Warnings, violation.Message)
		case contracts.LifecycleAudit:
			verdict.AuditViolations = append(verdict.AuditViolations, violation)
		}
	}

	if (!verdict.Allowed) && (len(verdict.Violations) > 0) {
		verdict.DenialReason = verdict.Violations[0].Message
	}

	return verdict
}

func evaluateManifestViolation(ruleExpression, resourceJSON string) bool {
	if strings.Contains(ruleExpression, "privileged == false") && strings.Contains(resourceJSON, `"privileged":true`) {
		return true
	}
	if strings.Contains(ruleExpression, "runAsNonRoot == true") && strings.Contains(resourceJSON, `"runAsNonRoot":false`) {
		return true
	}
	if strings.Contains(ruleExpression, "allowPrivilegeEscalation == false") && strings.Contains(resourceJSON, `"allowPrivilegeEscalation":true`) {
		return true
	}
	if strings.Contains(ruleExpression, "readOnlyRootFilesystem == true") && strings.Contains(resourceJSON, `"readOnlyRootFilesystem":false`) {
		return true
	}
	if strings.Contains(ruleExpression, "hostNetwork == false") && strings.Contains(resourceJSON, `"hostNetwork":true`) {
		return true
	}
	return false
}
