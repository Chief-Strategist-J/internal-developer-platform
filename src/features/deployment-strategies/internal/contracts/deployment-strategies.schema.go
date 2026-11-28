/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES INGRESS SCHEMA VALIDATION (V.3 role 02, G-01)

1. Purpose:
   - Validates inbound DTOs at the ingress boundary before dispatching to domain services.
   - Enforces string normalization (btrim, non-empty, regex checks).
2. Invariants:
   - Rejects invalid data early; never repairs or mutates raw payloads.
   - Zero inline comments below this header.
*/

package contracts

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrEmptyTenantID       = errors.New("tenantId is required and cannot be empty")
	ErrEmptyPlanID         = errors.New("planId is required and cannot be empty")
	ErrEmptyWorkloadID     = errors.New("workloadId is required and cannot be empty")
	ErrEmptyTargetImage    = errors.New("targetImage is required and cannot be empty")
	ErrInvalidPublicID     = errors.New("publicId violates canonical format regex")
	ErrInvalidStrategyType = errors.New("unsupported strategy type")
	ErrEmptySteps          = errors.New("canary steps must contain at least one step")
	ErrInvalidStepWeight   = errors.New("canary step weight must be between 1 and 100")
	ErrNonMonotonicSteps   = errors.New("canary step weights must strictly ascend")

	publicIDRegex = regexp.MustCompile(`^d(plan|exec|anly)_[0-9a-f]{26}$`)
)

func ValidateStartRolloutInput(input StartRolloutInput) error {
	if strings.TrimSpace(input.TenantID) == "" {
		return ErrEmptyTenantID
	}
	if strings.TrimSpace(input.PlanID) == "" {
		return ErrEmptyPlanID
	}
	if strings.TrimSpace(input.WorkloadID) == "" {
		return ErrEmptyWorkloadID
	}
	if strings.TrimSpace(input.TargetImage) == "" {
		return ErrEmptyTargetImage
	}
	return nil
}

func ValidateRolloutPlan(plan RolloutPlan) error {
	if strings.TrimSpace(plan.TenantID) == "" {
		return ErrEmptyTenantID
	}
	if strings.TrimSpace(plan.Name) == "" {
		return errors.New("plan name cannot be empty")
	}
	if plan.PublicID != "" && !publicIDRegex.MatchString(plan.PublicID) {
		return fmt.Errorf("%w: %s", ErrInvalidPublicID, plan.PublicID)
	}
	switch plan.StrategyType {
	case StrategyTypeCanary, StrategyTypeBlueGreen, StrategyTypeProgressiveMesh:
	default:
		return fmt.Errorf("%w: %s", ErrInvalidStrategyType, plan.StrategyType)
	}
	if plan.StrategyType == StrategyTypeCanary {
		if len(plan.Steps) == 0 {
			return ErrEmptySteps
		}
		previousWeight := 0
		for idx, step := range plan.Steps {
			if step.WeightPct < 1 || step.WeightPct > 100 {
				return fmt.Errorf("%w at index %d: %d", ErrInvalidStepWeight, idx, step.WeightPct)
			}
			if step.WeightPct <= previousWeight {
				return fmt.Errorf("%w: step %d (%d) <= step %d (%d)", ErrNonMonotonicSteps, idx, step.WeightPct, idx-1, previousWeight)
			}
			previousWeight = step.WeightPct
		}
	}
	return nil
}
