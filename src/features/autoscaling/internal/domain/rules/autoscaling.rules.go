/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING DOMAIN RULES (V.3 role 07, AS-INV-01..21)

1. Purpose:
   - Pure decision functions, one per invariant, each returning zero or more Violations:
       AS-INV-01 one replica writer            AS-INV-02 tier floor
       AS-INV-03 explicit ceilings chain       AS-INV-04 missing signal is not no-load
       AS-INV-05 requests always set           AS-INV-06 HPA and VPA separated
       AS-INV-07 Spot only for tolerant        AS-INV-08 shutdown fits the grace window
       AS-INV-09 reserved capacity             AS-INV-10 IP space planned for max scale
       AS-INV-11 scale-down safety             AS-INV-12 canary-aware scaling
       AS-INV-13 no MIG autoscaler             AS-INV-14 budget-bounded
       AS-INV-15 auditable                     AS-INV-16 database connection ceiling
       AS-INV-17 no unattended prod db scale   AS-INV-18 bounded storage increase limit
       AS-INV-19 kafka partition capped scale  AS-INV-20 pubsub consumer floor & dual metrics
       AS-INV-21 transaction pooler compatible
       AS-INV-22 scale-to-zero safety (doc AS-19)   AS-INV-23 ingestion signal & dead-letter (doc AS-20)
   - EvaluateAll runs every rule in a fixed order and aggregates; the plan is allowed only when
     no violation is returned.
2. Invariants:
   - Pure deterministic functions; zero IO, clocks, randomness or shared state.
   - Tier floors and ceilings arrive as arguments; nothing is defaulted here.
   - Zero inline comments below this header.
*/

package rules

import (
	"fmt"

	"internal-developer-platform/src/features/autoscaling/internal/contracts"
)

func mapToViolation(invariant, code string, severity contracts.ViolationSeverity, message string) contracts.Violation {
	return contracts.Violation{Invariant: invariant, Code: code, Severity: severity, Message: message}
}

func EvaluateOneReplicaWriter(plan contracts.ScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	active := 0
	for _, writer := range plan.ReplicaWriters {
		if writer != contracts.ReplicaWriterNone {
			active++
		}
	}
	if active > 1 {
		out = append(out, mapToViolation("AS-INV-01", "MULTIPLE_REPLICA_WRITERS", contracts.ViolationSeverityPage,
			fmt.Sprintf("%d autoscalers target one workload; exactly one replica writer is allowed", active)))
	}
	if active > 0 && plan.ReplicasPinnedInGit {
		out = append(out, mapToViolation("AS-INV-01", "REPLICAS_PINNED_IN_GIT", contracts.ViolationSeverityPage,
			"spec.replicas is committed in Git while an autoscaler owns replicas"))
	}
	return out
}

func evaluateProductionFloor(plan contracts.ScalingPlan, policy contracts.TierPolicy) []contracts.Violation {
	var out []contracts.Violation
	if plan.Environment != contracts.EnvironmentProduction {
		return out
	}
	if plan.MinReplicas < policy.MinReplicasFloor {
		out = append(out, mapToViolation("AS-INV-02", "MIN_REPLICAS_BELOW_TIER_FLOOR", contracts.ViolationSeverityPage,
			fmt.Sprintf("minReplicas %d is below the tier %s floor of %d", plan.MinReplicas, policy.Tier, policy.MinReplicasFloor)))
	}
	if plan.Zones < policy.MinZones {
		out = append(out, mapToViolation("AS-INV-02", "INSUFFICIENT_ZONES_FOR_TIER", contracts.ViolationSeverityPage,
			fmt.Sprintf("zones %d is below the tier %s requirement of %d", plan.Zones, policy.Tier, policy.MinZones)))
	}
	if policy.TopologySpread && !plan.TopologySpreadDeclared {
		out = append(out, mapToViolation("AS-INV-02", "MISSING_TOPOLOGY_SPREAD", contracts.ViolationSeverityTicket,
			fmt.Sprintf("tier %s requires explicit topologySpreadConstraints", policy.Tier)))
	}
	return out
}

func EvaluateTierFloor(plan contracts.ScalingPlan, policy contracts.TierPolicy) []contracts.Violation {
	out := evaluateProductionFloor(plan, policy)
	if plan.ScaleToZero && !policy.ScaleToZero {
		out = append(out, mapToViolation("AS-INV-02", "SCALE_TO_ZERO_FORBIDDEN_FOR_TIER", contracts.ViolationSeverityPage,
			fmt.Sprintf("scale-to-zero is not permitted on tier %s", policy.Tier)))
	}
	if plan.ScaleToZero && plan.SynchronousAPI && plan.Environment == contracts.EnvironmentProduction {
		out = append(out, mapToViolation("AS-INV-02", "SYNCHRONOUS_PROD_SCALE_TO_ZERO", contracts.ViolationSeverityPage,
			"synchronous citizen-facing services in production must never scale to zero"))
	}
	return out
}

type ceilingLimit struct {
	code  string
	label string
	limit int64
}

func EvaluateCeilingsChain(plan contracts.ScalingPlan, ceilings contracts.CapacityCeilings) []contracts.Violation {
	var out []contracts.Violation
	peak := countPeakPods(plan)
	if plan.Requests.CPUMilli > 0 && ceilings.QuotaCPUMilli > 0 && peak*plan.Requests.CPUMilli > ceilings.QuotaCPUMilli {
		out = append(out, mapToViolation("AS-INV-03", "MAX_SCALE_EXCEEDS_CPU_QUOTA", contracts.ViolationSeverityPage,
			fmt.Sprintf("peak pods %d requires %dm cpu; quota is %dm", peak, peak*plan.Requests.CPUMilli, ceilings.QuotaCPUMilli)))
	}
	limits := []ceilingLimit{
		{"MAX_SCALE_EXCEEDS_POD_QUOTA", "pod quota", ceilings.QuotaPods},
		{"MAX_SCALE_EXCEEDS_IP_HEADROOM", "namespace IP headroom", ceilings.IPHeadroomPods},
		{"MAX_SCALE_EXCEEDS_NODE_POOL_CAPACITY", "node pool capacity", ceilings.NodePoolMaxPods},
		{"MAX_SCALE_EXCEEDS_BUDGET_CAP", "approved budget cap", ceilings.BudgetMaxPods},
	}
	for _, c := range limits {
		if c.limit > 0 && peak > c.limit {
			out = append(out, mapToViolation("AS-INV-03", c.code, contracts.ViolationSeverityPage,
				fmt.Sprintf("peak pods %d (max %d + surge %d + canary %d) exceed %s %d",
					peak, plan.MaxReplicas, plan.RolloutSurgePods, plan.CanaryPods, c.label, c.limit)))
		}
	}
	return out
}

func countPeakPods(plan contracts.ScalingPlan) int64 {
	return int64(plan.MaxReplicas) + int64(plan.RolloutSurgePods) + int64(plan.CanaryPods)
}

func EvaluateSignalNotIdle(signal contracts.SignalObservation) []contracts.Violation {
	var out []contracts.Violation
	if signal.State == contracts.SignalStateUnavailable && signal.ProposedReplicas < signal.CurrentReplicas {
		out = append(out, mapToViolation("AS-INV-04", "SCALEDOWN_ON_UNAVAILABLE_SIGNAL", contracts.ViolationSeverityPage,
			"scale-down is forbidden while the scaling signal is unavailable; hold current capacity"))
	}
	return out
}

func EvaluateRequestsSet(plan contracts.ScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if plan.Requests.CPUMilli <= 0 {
		out = append(out, mapToViolation("AS-INV-05", "MISSING_CPU_REQUEST", contracts.ViolationSeverityPage,
			"containers must declare a positive cpu request; autoscaling cannot function without it"))
	}
	if plan.Requests.MemoryBytes <= 0 {
		out = append(out, mapToViolation("AS-INV-05", "MISSING_MEMORY_REQUEST", contracts.ViolationSeverityPage,
			"containers must declare a positive memory request; autoscaling cannot function without it"))
	}
	return out
}

func EvaluateHPAVPASeparation(plan contracts.ScalingPlan, policy contracts.TierPolicy) []contracts.Violation {
	var out []contracts.Violation
	if plan.Vertical.Mode == contracts.VPAModeInPlaceOrRecreate {
		if policy.TierCritical {
			out = append(out, mapToViolation("AS-INV-06", "VPA_IN_PLACE_FORBIDDEN_ON_TIER_CRITICAL", contracts.ViolationSeverityPage,
				"VPA in-place mode is beta; tier-critical workloads must use RECOMMEND_ONLY"))
		}
		for _, vpaMetric := range plan.Vertical.Metrics {
			for _, hpaMetric := range plan.HorizontalMetrics {
				if vpaMetric == hpaMetric {
					out = append(out, mapToViolation("AS-INV-06", "HPA_VPA_METRIC_COLLISION", contracts.ViolationSeverityPage,
						fmt.Sprintf("HPA and VPA cannot target the same metric: %s", vpaMetric)))
				}
			}
		}
	}
	return out
}

func EvaluateSpotPlacement(policy contracts.TierPolicy, spot contracts.SpotPlacement) []contracts.Violation {
	var out []contracts.Violation
	if spot.OnSpot && policy.TierCritical {
		out = append(out, mapToViolation("AS-INV-07", "TIER_CRITICAL_ON_SPOT", contracts.ViolationSeverityPage,
			"tier-critical workloads must never run on Spot VMs"))
	}
	if spot.OnSpot && !spot.ClassAllowedOnSpot {
		out = append(out, mapToViolation("AS-INV-07", "DISALLOWED_CLASS_ON_SPOT", contracts.ViolationSeverityPage,
			"workload class is not approved for Spot execution"))
	}
	if spot.OnSpot && !spot.StandardNonGPUPoolPresent {
		out = append(out, mapToViolation("AS-INV-07", "NO_STANDARD_FALLBACK_POOL", contracts.ViolationSeverityPage,
			"spot pools require at least one standard non-GPU fallback pool in the cluster"))
	}
	if spot.OnSpot {
		if spot.ShutdownSeconds > spot.EffectiveGraceSeconds {
			out = append(out, mapToViolation("AS-INV-08", "SHUTDOWN_EXCEEDS_SPOT_NOTICE", contracts.ViolationSeverityPage,
				fmt.Sprintf("shutdown duration %ds exceeds effective Spot grace %ds",
					spot.ShutdownSeconds, spot.EffectiveGraceSeconds)))
		}
		if spot.Stateful && spot.ShutdownSeconds > spot.StatefulBudgetSeconds {
			out = append(out, mapToViolation("AS-INV-08", "STATEFUL_DRAIN_EXCEEDS_BUDGET", contracts.ViolationSeverityPage,
				fmt.Sprintf("stateful workload drain %ds exceeds Spot budget %ds",
					spot.ShutdownSeconds, spot.StatefulBudgetSeconds)))
		}
	}
	return out
}

func EvaluateReservedCapacity(policy contracts.TierPolicy, reservation contracts.ReservedCapacity) []contracts.Violation {
	var out []contracts.Violation
	if !policy.TierCritical {
		return out
	}
	if !reservation.Reserved {
		out = append(out, mapToViolation("AS-INV-09", "NO_RESERVED_CAPACITY", contracts.ViolationSeverityPage,
			"tier-critical scale-up requires reserved capacity"))
	}
	if !reservation.FallbackClassesListed {
		out = append(out, mapToViolation("AS-INV-09", "NO_FALLBACK_CLASSES", contracts.ViolationSeverityTicket,
			"compute class fallbacks must be listed for tier-critical pools"))
	}
	return out
}

func EvaluateIPPlan(ip contracts.IPPlan) []contracts.Violation {
	var out []contracts.Violation
	if ip.NodePoolMaxNodes*ip.PodsPerNodeRange > ip.PodRangeSize {
		out = append(out, mapToViolation("AS-INV-10", "POD_RANGE_TOO_SMALL", contracts.ViolationSeverityPage,
			fmt.Sprintf("node pool max %d x per-node range %d exceeds pod range %d",
				ip.NodePoolMaxNodes, ip.PodsPerNodeRange, ip.PodRangeSize)))
	}
	return out
}

func EvaluateScaleDownSafety(disruption contracts.DisruptionPolicy) []contracts.Violation {
	var out []contracts.Violation
	if disruption.AllowedDisruptions < 1 {
		out = append(out, mapToViolation("AS-INV-11", "PDB_BLOCKS_DISRUPTION", contracts.ViolationSeverityTicket,
			"PodDisruptionBudget must allow at least one disruption"))
	}
	if disruption.UnhealthyPodEviction != "AlwaysAllow" {
		out = append(out, mapToViolation("AS-INV-11", "UNHEALTHY_EVICTION_NOT_ALWAYS_ALLOW", contracts.ViolationSeverityTicket,
			"unhealthyPodEvictionPolicy must be AlwaysAllow"))
	}
	if disruption.BarePods > 0 {
		out = append(out, mapToViolation("AS-INV-11", "BARE_PODS_PRESENT", contracts.ViolationSeverityTicket,
			fmt.Sprintf("%d bare pods block node scale-down", disruption.BarePods)))
	}
	return out
}

func EvaluateCanaryAwareScaling(canary contracts.CanaryScaling) []contracts.Violation {
	var out []contracts.Violation
	if canary.Enabled && !canary.AnalysisPerRevision {
		out = append(out, mapToViolation("AS-INV-12", "CANARY_ANALYSIS_NOT_PER_REVISION", contracts.ViolationSeverityTicket,
			"canary analysis must compare per-revision metrics, not HPA averages"))
	}
	if canary.DynamicStableScale && !canary.TrafficRouting {
		out = append(out, mapToViolation("AS-INV-12", "DYNAMIC_STABLE_SCALE_WITHOUT_ROUTING", contracts.ViolationSeverityTicket,
			"dynamicStableScale requires traffic routing"))
	}
	return out
}

func EvaluateNoExternalAutoscaler(migAutoscalerEnabled bool) []contracts.Violation {
	var out []contracts.Violation
	if migAutoscalerEnabled {
		out = append(out, mapToViolation("AS-INV-13", "MIG_AUTOSCALER_ENABLED", contracts.ViolationSeverityTicket,
			"Compute Engine autoscaling on node MIGs conflicts with the cluster autoscaler"))
	}
	return out
}

func EvaluateBudget(budget contracts.Budget) []contracts.Violation {
	var out []contracts.Violation
	if budget.ProjectedMaxSpendMinor > budget.TenantBudgetMinor && !budget.OverrunApproved {
		out = append(out, mapToViolation("AS-INV-14", "BUDGET_OVERRUN_UNAPPROVED", contracts.ViolationSeverityTicket,
			fmt.Sprintf("projected max spend %d %s exceeds tenant budget %d without approval",
				budget.ProjectedMaxSpendMinor, budget.Currency, budget.TenantBudgetMinor)))
	}
	return out
}

func EvaluateAuditability(eventsRetained bool) []contracts.Violation {
	var out []contracts.Violation
	if !eventsRetained {
		out = append(out, mapToViolation("AS-INV-15", "SCALING_EVENTS_NOT_RETAINED", contracts.ViolationSeverityTicket,
			"scaling events must be retained with workload, reason and before/after"))
	}
	return out
}

func EvaluateDatabaseConnectionCeiling(plan contracts.ScalingPlan, db contracts.DatabaseScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if db.TalksToDatabase && db.PerPodPoolSize > 0 {
		projected := int64(plan.MaxReplicas) * int64(db.PerPodPoolSize)
		if db.DBConnectionBudget > 0 && projected > int64(db.DBConnectionBudget) {
			out = append(out, mapToViolation("AS-INV-16", "DB_CONNECTION_CEILING_EXCEEDED", contracts.ViolationSeverityPage,
				fmt.Sprintf("projected connections %d (maxReplicas %d x pool %d) exceeds database connection budget %d",
					projected, plan.MaxReplicas, db.PerPodPoolSize, db.DBConnectionBudget)))
		}
	}
	return out
}

func EvaluateNoAutomatedDatabaseScaling(plan contracts.ScalingPlan, db contracts.DatabaseScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if db.AutomatedVerticalScaleRequested && plan.Environment == contracts.EnvironmentProduction {
		if !db.InScheduledMaintenanceWindow || !db.OperatorApprovalRecorded {
			out = append(out, mapToViolation("AS-INV-17", "UNATTENDED_PROD_DB_SCALING", contracts.ViolationSeverityTicket,
				"automated vertical database scaling in production is forbidden without a scheduled maintenance window and operator approval"))
		}
	}
	return out
}

func EvaluateStorageIncreaseLimit(db contracts.DatabaseScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if db.TalksToDatabase && db.StorageAutoIncreaseLimit <= 0 {
		out = append(out, mapToViolation("AS-INV-18", "UNBOUNDED_STORAGE_GROWTH", contracts.ViolationSeverityTicket,
			"database storage automatic increase limit must be explicitly bounded (> 0)"))
	}
	return out
}

func EvaluateKafkaPartitionCapping(ingest contracts.IngestionScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if ingest.Backbone == contracts.BackboneKafka && ingest.TopicPartitions > 0 {
		if ingest.ConsumerMaxReplicas > ingest.TopicPartitions {
			out = append(out, mapToViolation("AS-INV-19", "KAFKA_REPLICAS_EXCEED_PARTITIONS", contracts.ViolationSeverityTicket,
				fmt.Sprintf("kafka consumer maxReplicas %d exceeds topic partition count %d",
					ingest.ConsumerMaxReplicas, ingest.TopicPartitions)))
		}
	}
	return out
}

func EvaluatePubSubConsumerFloor(plan contracts.ScalingPlan, ingest contracts.IngestionScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if ingest.Backbone == contracts.BackbonePubSub && plan.Environment == contracts.EnvironmentProduction {
		if ingest.ConsumerMinReplicas < 1 {
			out = append(out, mapToViolation("AS-INV-20", "PUBSUB_SCALE_TO_ZERO_IN_PROD", contracts.ViolationSeverityPage,
				"production pubsub consumers must maintain minReplicas >= 1 to prevent cold-start starvation"))
		}
		if !ingest.DualMetricConfigured {
			out = append(out, mapToViolation("AS-INV-20", "PUBSUB_SINGLE_METRIC_SCALING", contracts.ViolationSeverityTicket,
				"pubsub scaling requires dual-metric configuration (backlog/age + CPU)"))
		}
	}
	return out
}

func EvaluateTransactionPoolingCompatibility(db contracts.DatabaseScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if db.UsesTransactionPooler && db.RequiresSessionState {
		out = append(out, mapToViolation("AS-INV-21", "TRANSACTION_POOLING_INCOMPATIBLE", contracts.ViolationSeverityPage,
			"workload requiring session features (LISTEN/NOTIFY, temp tables, cursors) cannot use transaction-mode connection pooler"))
	}
	return out
}

func evaluatePlanRules(input contracts.EvaluatePlanInput) []contracts.Violation {
	var out []contracts.Violation
	out = append(out, EvaluateOneReplicaWriter(input.Plan)...)
	out = append(out, EvaluateTierFloor(input.Plan, input.Policy)...)
	out = append(out, EvaluateCeilingsChain(input.Plan, input.Ceilings)...)
	out = append(out, EvaluateSignalNotIdle(input.Signal)...)
	out = append(out, EvaluateRequestsSet(input.Plan)...)
	out = append(out, EvaluateHPAVPASeparation(input.Plan, input.Policy)...)
	return out
}

func evaluateClusterRules(input contracts.EvaluatePlanInput) []contracts.Violation {
	cluster := input.Cluster
	var out []contracts.Violation
	out = append(out, EvaluateSpotPlacement(input.Policy, cluster.Spot)...)
	out = append(out, EvaluateReservedCapacity(input.Policy, cluster.Reservation)...)
	out = append(out, EvaluateIPPlan(cluster.IP)...)
	out = append(out, EvaluateScaleDownSafety(cluster.Disruption)...)
	out = append(out, EvaluateCanaryAwareScaling(cluster.Canary)...)
	out = append(out, EvaluateNoExternalAutoscaler(cluster.MIGAutoscalerEnabled)...)
	out = append(out, EvaluateBudget(cluster.Budget)...)
	out = append(out, EvaluateAuditability(cluster.ScalingEventsRetained)...)
	return out
}

func evaluateDataRules(input contracts.EvaluatePlanInput) []contracts.Violation {
	cluster := input.Cluster
	var out []contracts.Violation
	out = append(out, EvaluateDatabaseConnectionCeiling(input.Plan, cluster.Database)...)
	out = append(out, EvaluateNoAutomatedDatabaseScaling(input.Plan, cluster.Database)...)
	out = append(out, EvaluateStorageIncreaseLimit(cluster.Database)...)
	out = append(out, EvaluateKafkaPartitionCapping(cluster.Ingestion)...)
	out = append(out, EvaluatePubSubConsumerFloor(input.Plan, cluster.Ingestion)...)
	out = append(out, EvaluateIngestionSignal(input.Plan, cluster.Ingestion)...)
	out = append(out, EvaluateScaleToZeroSafety(input.Plan, cluster.Database, cluster.Serverless)...)
	out = append(out, EvaluateTransactionPoolingCompatibility(cluster.Database)...)
	return out
}

func EvaluateAll(input contracts.EvaluatePlanInput) contracts.Evaluation {
	violations := make([]contracts.Violation, 0)
	violations = append(violations, evaluatePlanRules(input)...)
	violations = append(violations, evaluateClusterRules(input)...)
	violations = append(violations, evaluateDataRules(input)...)
	return contracts.Evaluation{Allowed: len(violations) == 0, Violations: violations}
}
