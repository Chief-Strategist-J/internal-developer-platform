/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY-AS-CODE DOMAIN RULES

1. Architecture Role:
   - Step 04 Domain Rules: Pure domain invariants for exception eligibility and CEL validation (§2 & §6 in business-logic.md).
   - Enforces the strict non-exceptable rule blacklist:
     * POD-01..09 (PSS Restricted Baseline)
     * IMG-01..03 (Provenance & Digest Pinning)
     * RBAC-01..04 (IdP Group Authorization & Escalate Prevention)
     * NET-02..03 (mTLS STRICT & EnvoyFilter Ban)
     * TEN-02..03, TEN-06 (Cluster Scope & Exemption Label Guard)
     * CLD-* (Cloud Organization Invariants)
   - Adheres strictly to 06-writing-conditions.md (parenthesized compound conditions, half-open intervals).
   - Zero inline comments inside method bodies; pure deterministic domain checks.
*/
package rules

import (
	"fmt"
	"strings"
	"time"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/shared/errors"
)

const (
	MaxExceptionTTL = 90 * 24 * time.Hour
)

var nonExceptableRuleSet = map[string]bool{
	"POD-01": true, "POD-02": true, "POD-03": true, "POD-04": true,
	"POD-05": true, "POD-06": true, "POD-07": true, "POD-08": true, "POD-09": true,
	"IMG-01": true, "IMG-02": true, "IMG-03": true,
	"RBAC-01": true, "RBAC-02": true, "RBAC-03": true, "RBAC-04": true,
	"NET-02": true, "NET-03": true,
	"TEN-02": true, "TEN-03": true, "TEN-06": true,
}

func IsRuleExceptable(externalID string) bool {
	cleanID := strings.ToUpper(strings.TrimSpace(externalID))
	if cleanID == "" {
		return false
	}
	if strings.HasPrefix(cleanID, "CLD-") {
		return false
	}
	if nonExceptableRuleSet[cleanID] {
		return false
	}
	return true
}

func ValidateExceptionEligibility(
	policy contracts.PolicyRecord,
	req contracts.PolicyExceptionRecord,
	now time.Time,
) error {
	if (!policy.Exceptable) || (!IsRuleExceptable(policy.ExternalID)) {
		return errors.NewValidationError(
			fmt.Sprintf("policy %s (%s) is non-exceptable under platform governance baseline", policy.ExternalID, policy.Name),
		)
	}

	if req.ApproverID == req.TenantID {
		return errors.NewValidationError("separation of duties violation: approver cannot be the requesting tenant identity")
	}

	duration := req.ExpiresAt.Sub(now)
	if duration <= 0 {
		return errors.NewValidationError("exception expiration must be strictly in the future")
	}
	if duration > MaxExceptionTTL {
		return errors.NewValidationError(
			fmt.Sprintf("exception duration exceeds maximum permitted lease of %d days", int(MaxExceptionTTL.Hours()/24)),
		)
	}

	return nil
}

func ValidateCELRuleExpression(expr string) error {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return errors.NewValidationError("CEL rule expression cannot be empty")
	}
	if len(trimmed) > 4096 {
		return errors.NewValidationError("CEL rule expression exceeds maximum limit of 4096 characters")
	}

	parenCount := 0
	bracketCount := 0
	braceCount := 0

	for _, ch := range trimmed {
		switch ch {
		case '(':
			parenCount++
		case ')':
			parenCount--
		case '[':
			bracketCount++
		case ']':
			bracketCount--
		case '{':
			braceCount++
		case '}':
			braceCount--
		}
		if (parenCount < 0) || (bracketCount < 0) || (braceCount < 0) {
			return errors.NewValidationError("CEL expression has unbalanced closing delimiter")
		}
	}

	if (parenCount != 0) || (bracketCount != 0) || (braceCount != 0) {
		return errors.NewValidationError("CEL expression has unclosed delimiter")
	}

	return nil
}
