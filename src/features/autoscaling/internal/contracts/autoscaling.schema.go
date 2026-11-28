/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING INGRESS SCHEMA VALIDATION (V.3 role 03, G-01)

1. Purpose:
   - Strict-parse gate for EvaluatePlanInput before any guardrail runs: rejects, never repairs.
   - EvaluateInputSchema runs the identity, bounds, enum and quantity checks in a fixed order.
2. Invariants:
   - Rejects empty identifiers, unknown enum values, negative quantities and inverted bounds.
   - Each check is a small pure function; none exceeds the 30-line or complexity-10 limits.
   - Zero inline comments below this header.
*/

package contracts

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrEmptyTenantID       = errors.New("tenant context is required and cannot be empty")
	ErrEmptyWorkload       = errors.New("workload namespace, name and kind are required")
	ErrEmptyTier           = errors.New("tier is required and must match the tier policy")
	ErrInvalidEnvironment  = errors.New("unsupported environment")
	ErrInvalidReplicaBound = errors.New("minReplicas must be >= 0 and maxReplicas must be >= minReplicas and >= 1")
	ErrInvalidWriter       = errors.New("unsupported replica writer")
	ErrInvalidVPAMode      = errors.New("unsupported vpa mode")
	ErrInvalidSignalState  = errors.New("unsupported signal state")
	ErrNegativeQuantity    = errors.New("quantities must not be negative")
	ErrInvalidCurrency     = errors.New("budget currency must be a 3-letter code")
	ErrInvalidIngestion    = errors.New("unsupported ingestion backbone or signal")
	ErrTierMismatch        = errors.New("plan tier does not match tier policy")
)

func isAnyNegative(values ...int64) bool {
	for _, value := range values {
		if value < 0 {
			return true
		}
	}
	return false
}

func isKnownEnvironment(value Environment) bool {
	switch value {
	case EnvironmentProduction, EnvironmentNonProduction:
		return true
	}
	return false
}

func isKnownWriter(value ReplicaWriter) bool {
	switch value {
	case ReplicaWriterHPA, ReplicaWriterKEDA, ReplicaWriterNone:
		return true
	}
	return false
}

func isKnownVPAMode(value VPAMode) bool {
	switch value {
	case VPAModeOff, VPAModeRecommendOnly, VPAModeInPlaceOrRecreate:
		return true
	}
	return false
}

func isKnownSignalState(value SignalState) bool {
	switch value {
	case SignalStateAvailable, SignalStateUnavailable:
		return true
	}
	return false
}

func isKnownBackbone(value string) bool {
	switch value {
	case "", BackboneKafka, BackbonePubSub:
		return true
	}
	return false
}

func evaluateIdentity(input EvaluatePlanInput) error {
	plan := input.Plan
	if strings.TrimSpace(plan.TenantID) == "" {
		return ErrEmptyTenantID
	}
	workload := plan.Workload
	if strings.TrimSpace(workload.Namespace) == "" || strings.TrimSpace(workload.Name) == "" || strings.TrimSpace(workload.Kind) == "" {
		return ErrEmptyWorkload
	}
	if strings.TrimSpace(plan.Tier) == "" {
		return ErrEmptyTier
	}
	if plan.Tier != input.Policy.Tier {
		return fmt.Errorf("%w: plan=%s policy=%s", ErrTierMismatch, plan.Tier, input.Policy.Tier)
	}
	return nil
}

func evaluateBounds(input EvaluatePlanInput) error {
	plan := input.Plan
	if plan.MinReplicas < 0 || plan.MaxReplicas < 1 || plan.MaxReplicas < plan.MinReplicas {
		return fmt.Errorf("%w: min=%d max=%d", ErrInvalidReplicaBound, plan.MinReplicas, plan.MaxReplicas)
	}
	if len(input.Cluster.Budget.Currency) != 3 {
		return ErrInvalidCurrency
	}
	return nil
}

func evaluateEnums(input EvaluatePlanInput) error {
	plan := input.Plan
	if !isKnownEnvironment(plan.Environment) {
		return fmt.Errorf("%w: %s", ErrInvalidEnvironment, plan.Environment)
	}
	for _, writer := range plan.ReplicaWriters {
		if !isKnownWriter(writer) {
			return fmt.Errorf("%w: %s", ErrInvalidWriter, writer)
		}
	}
	if !isKnownVPAMode(plan.Vertical.Mode) {
		return fmt.Errorf("%w: %s", ErrInvalidVPAMode, plan.Vertical.Mode)
	}
	if !isKnownSignalState(input.Signal.State) {
		return fmt.Errorf("%w: %s", ErrInvalidSignalState, input.Signal.State)
	}
	return nil
}

func isKnownIngestSignal(value string) bool {
	switch value {
	case "", IngestSignalBacklog, IngestSignalOldestAge, IngestSignalLag, IngestSignalAckRate, IngestSignalCPUOnly:
		return true
	}
	return false
}

func evaluateIngestion(input EvaluatePlanInput) error {
	ingestion := input.Cluster.Ingestion
	if !isKnownBackbone(ingestion.Backbone) {
		return fmt.Errorf("%w: %s", ErrInvalidIngestion, ingestion.Backbone)
	}
	if !isKnownIngestSignal(ingestion.Signal) {
		return fmt.Errorf("%w: signal %s", ErrInvalidIngestion, ingestion.Signal)
	}
	return nil
}

func evaluatePlanQuantities(input EvaluatePlanInput) error {
	plan := input.Plan
	signal := input.Signal
	if isAnyNegative(int64(plan.Zones), int64(plan.RolloutSurgePods), int64(plan.CanaryPods),
		plan.Requests.CPUMilli, plan.Requests.MemoryBytes,
		int64(signal.CurrentReplicas), int64(signal.ProposedReplicas)) {
		return ErrNegativeQuantity
	}
	return nil
}

func evaluateCeilingQuantities(input EvaluatePlanInput) error {
	ceilings := input.Ceilings
	if isAnyNegative(ceilings.QuotaCPUMilli, ceilings.QuotaPods, ceilings.NodePoolMaxPods,
		ceilings.IPHeadroomPods, ceilings.BudgetMaxPods) {
		return ErrNegativeQuantity
	}
	return nil
}

func evaluateClusterQuantities(input EvaluatePlanInput) error {
	cluster := input.Cluster
	if isAnyNegative(int64(cluster.Spot.ShutdownSeconds), int64(cluster.Spot.EffectiveGraceSeconds),
		int64(cluster.Spot.StatefulBudgetSeconds), cluster.IP.NodePoolMaxNodes, cluster.IP.PodsPerNodeRange,
		cluster.IP.PodRangeSize, int64(cluster.Disruption.AllowedDisruptions), int64(cluster.Disruption.BarePods),
		cluster.Budget.ProjectedMaxSpendMinor, cluster.Budget.TenantBudgetMinor) {
		return ErrNegativeQuantity
	}
	return nil
}

func evaluateDataQuantities(input EvaluatePlanInput) error {
	database := input.Cluster.Database
	ingestion := input.Cluster.Ingestion
	if isAnyNegative(int64(database.PerPodPoolSize), int64(database.DBConnectionBudget), database.StorageAutoIncreaseLimit,
		int64(ingestion.TopicPartitions), int64(ingestion.ConsumerMaxReplicas), int64(ingestion.ConsumerMinReplicas),
		int64(input.Cluster.Serverless.ColdStartSeconds), int64(input.Cluster.Serverless.ColdStartBudgetSeconds)) {
		return ErrNegativeQuantity
	}
	return nil
}

func EvaluateInputSchema(input EvaluatePlanInput) error {
	checks := []func(EvaluatePlanInput) error{
		evaluateIdentity, evaluateBounds, evaluateEnums, evaluateIngestion,
		evaluatePlanQuantities, evaluateCeilingQuantities, evaluateClusterQuantities, evaluateDataQuantities,
	}
	for _, check := range checks {
		if err := check(input); err != nil {
			return err
		}
	}
	return nil
}
