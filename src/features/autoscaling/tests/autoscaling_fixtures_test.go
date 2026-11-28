/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING TEST FIXTURES (V.3 role 14)

1. Purpose:
   - Shared builders for a fully compliant plan and the negative-case tables, one table per invariant group.
2. Invariants:
   - Pure builders; no mocks, no IO. Every function stays within 30 lines.
   - Zero inline comments below this header.
*/

package tests

import (
	"internal-developer-platform/src/features/autoscaling/internal/contracts"
)

type negativeCase struct {
	name   string
	mutate func(*contracts.EvaluatePlanInput)
	code   string
}

func compliantPlan() contracts.ScalingPlan {
	return contracts.ScalingPlan{
		TenantID:               "tenant-a",
		Workload:               contracts.WorkloadRef{Namespace: "app-prod", Name: "api", Kind: "Rollout"},
		Environment:            contracts.EnvironmentProduction,
		Tier:                   "gold",
		ReplicaWriters:         []contracts.ReplicaWriter{contracts.ReplicaWriterHPA},
		MinReplicas:            3,
		MaxReplicas:            10,
		Zones:                  3,
		TopologySpreadDeclared: true,
		SynchronousAPI:         true,
		HorizontalMetrics:      []string{"cpu"},
		Requests:               contracts.ContainerRequests{CPUMilli: 250, MemoryBytes: 256 << 20},
		Vertical:               contracts.VerticalScaling{Mode: contracts.VPAModeRecommendOnly, Metrics: []string{"memory"}},
		RolloutSurgePods:       2,
		CanaryPods:             1,
	}
}

func compliantCluster() contracts.ClusterState {
	return contracts.ClusterState{
		Spot:                  contracts.SpotPlacement{StandardNonGPUPoolPresent: true},
		Reservation:           contracts.ReservedCapacity{Reserved: true, FallbackClassesListed: true},
		IP:                    contracts.IPPlan{NodePoolMaxNodes: 10, PodsPerNodeRange: 32, PodRangeSize: 1024},
		Disruption:            contracts.DisruptionPolicy{AllowedDisruptions: 1, UnhealthyPodEviction: "AlwaysAllow"},
		Canary:                contracts.CanaryScaling{Enabled: true, AnalysisPerRevision: true, TrafficRouting: true},
		Budget:                contracts.Budget{Currency: "USD", ProjectedMaxSpendMinor: 1000, TenantBudgetMinor: 2000},
		ScalingEventsRetained: true,
	}
}

func compliantInput() contracts.EvaluatePlanInput {
	return contracts.EvaluatePlanInput{
		Plan:     compliantPlan(),
		Policy:   contracts.TierPolicy{Tier: "gold", MinReplicasFloor: 3, MinZones: 3, TopologySpread: true, TierCritical: true},
		Ceilings: contracts.CapacityCeilings{QuotaCPUMilli: 10000, QuotaPods: 20, NodePoolMaxPods: 40, IPHeadroomPods: 60, BudgetMaxPods: 80},
		Signal:   contracts.SignalObservation{State: contracts.SignalStateAvailable, CurrentReplicas: 5, ProposedReplicas: 4},
		Cluster:  compliantCluster(),
	}
}

func writerCases() []negativeCase {
	return []negativeCase{
		{"two replica writers", func(i *contracts.EvaluatePlanInput) {
			i.Plan.ReplicaWriters = []contracts.ReplicaWriter{contracts.ReplicaWriterHPA, contracts.ReplicaWriterKEDA}
		}, "MULTIPLE_REPLICA_WRITERS"},
		{"replicas pinned in git", func(i *contracts.EvaluatePlanInput) { i.Plan.ReplicasPinnedInGit = true }, "REPLICAS_PINNED_IN_GIT"},
		{"requests missing", func(i *contracts.EvaluatePlanInput) { i.Plan.Requests.CPUMilli = 0 }, "MISSING_CPU_REQUEST"},
		{"hpa and vpa share metric", func(i *contracts.EvaluatePlanInput) {
			i.Plan.Vertical = contracts.VerticalScaling{Mode: contracts.VPAModeInPlaceOrRecreate, Metrics: []string{"cpu"}}
		}, "HPA_VPA_METRIC_COLLISION"},
		{"in-place vpa on tier critical", func(i *contracts.EvaluatePlanInput) { i.Plan.Vertical.Mode = contracts.VPAModeInPlaceOrRecreate }, "VPA_IN_PLACE_FORBIDDEN_ON_TIER_CRITICAL"},
	}
}

func tierCases() []negativeCase {
	return []negativeCase{
		{"below tier floor", func(i *contracts.EvaluatePlanInput) { i.Plan.MinReplicas = 1 }, "MIN_REPLICAS_BELOW_TIER_FLOOR"},
		{"below zone floor", func(i *contracts.EvaluatePlanInput) { i.Plan.Zones = 1 }, "INSUFFICIENT_ZONES_FOR_TIER"},
		{"topology spread missing", func(i *contracts.EvaluatePlanInput) { i.Plan.TopologySpreadDeclared = false }, "MISSING_TOPOLOGY_SPREAD"},
		{"scale to zero forbidden", func(i *contracts.EvaluatePlanInput) { i.Plan.ScaleToZero = true }, "SCALE_TO_ZERO_FORBIDDEN_FOR_TIER"},
		{"scale to zero sync api", func(i *contracts.EvaluatePlanInput) {
			i.Plan.ScaleToZero = true
			i.Policy.ScaleToZero = true
		}, "SYNCHRONOUS_PROD_SCALE_TO_ZERO"},
	}
}

func ceilingCases() []negativeCase {
	return []negativeCase{
		{"pods exceed ceiling", func(i *contracts.EvaluatePlanInput) { i.Plan.MaxReplicas = 25 }, "MAX_SCALE_EXCEEDS_POD_QUOTA"},
		{"cpu exceeds quota", func(i *contracts.EvaluatePlanInput) { i.Ceilings.QuotaCPUMilli = 1000 }, "MAX_SCALE_EXCEEDS_CPU_QUOTA"},
		{"node pool capacity exceeded", func(i *contracts.EvaluatePlanInput) { i.Ceilings.NodePoolMaxPods = 5 }, "MAX_SCALE_EXCEEDS_NODE_POOL_CAPACITY"},
		{"ip headroom exceeded", func(i *contracts.EvaluatePlanInput) { i.Ceilings.IPHeadroomPods = 5 }, "MAX_SCALE_EXCEEDS_IP_HEADROOM"},
		{"budget cap exceeded", func(i *contracts.EvaluatePlanInput) { i.Ceilings.BudgetMaxPods = 5 }, "MAX_SCALE_EXCEEDS_BUDGET_CAP"},
		{"scale down on missing signal", func(i *contracts.EvaluatePlanInput) { i.Signal.State = contracts.SignalStateUnavailable }, "SCALEDOWN_ON_UNAVAILABLE_SIGNAL"},
		{"pod range too small", func(i *contracts.EvaluatePlanInput) { i.Cluster.IP.PodRangeSize = 100 }, "POD_RANGE_TOO_SMALL"},
		{"budget overrun", func(i *contracts.EvaluatePlanInput) { i.Cluster.Budget.ProjectedMaxSpendMinor = 3000 }, "BUDGET_OVERRUN_UNAPPROVED"},
	}
}

func spotCases() []negativeCase {
	return []negativeCase{
		{"tier critical on spot", func(i *contracts.EvaluatePlanInput) { i.Cluster.Spot.OnSpot = true }, "TIER_CRITICAL_ON_SPOT"},
		{"class not allowed on spot", func(i *contracts.EvaluatePlanInput) {
			i.Policy.TierCritical = false
			i.Cluster.Spot.OnSpot = true
		}, "DISALLOWED_CLASS_ON_SPOT"},
		{"no standard pool", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Spot.OnSpot = true
			i.Cluster.Spot.StandardNonGPUPoolPresent = false
		}, "NO_STANDARD_FALLBACK_POOL"},
		{"shutdown exceeds grace", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Spot = contracts.SpotPlacement{OnSpot: true, ClassAllowedOnSpot: true, StandardNonGPUPoolPresent: true, ShutdownSeconds: 20, EffectiveGraceSeconds: 15}
		}, "SHUTDOWN_EXCEEDS_SPOT_NOTICE"},
		{"stateful shutdown exceeds budget", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Spot = contracts.SpotPlacement{OnSpot: true, ClassAllowedOnSpot: true, StandardNonGPUPoolPresent: true, Stateful: true, ShutdownSeconds: 20, EffectiveGraceSeconds: 30, StatefulBudgetSeconds: 10}
		}, "STATEFUL_DRAIN_EXCEEDS_BUDGET"},
		{"no reserved capacity", func(i *contracts.EvaluatePlanInput) { i.Cluster.Reservation.Reserved = false }, "NO_RESERVED_CAPACITY"},
		{"no fallback classes", func(i *contracts.EvaluatePlanInput) { i.Cluster.Reservation.FallbackClassesListed = false }, "NO_FALLBACK_CLASSES"},
	}
}

func operationsCases() []negativeCase {
	return []negativeCase{
		{"pdb blocks disruption", func(i *contracts.EvaluatePlanInput) { i.Cluster.Disruption.AllowedDisruptions = 0 }, "PDB_BLOCKS_DISRUPTION"},
		{"unhealthy eviction", func(i *contracts.EvaluatePlanInput) { i.Cluster.Disruption.UnhealthyPodEviction = "IfHealthyBudget" }, "UNHEALTHY_EVICTION_NOT_ALWAYS_ALLOW"},
		{"bare pods", func(i *contracts.EvaluatePlanInput) { i.Cluster.Disruption.BarePods = 2 }, "BARE_PODS_PRESENT"},
		{"canary analysis not per revision", func(i *contracts.EvaluatePlanInput) { i.Cluster.Canary.AnalysisPerRevision = false }, "CANARY_ANALYSIS_NOT_PER_REVISION"},
		{"dynamic stable scale without routing", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Canary.DynamicStableScale = true
			i.Cluster.Canary.TrafficRouting = false
		}, "DYNAMIC_STABLE_SCALE_WITHOUT_ROUTING"},
		{"mig autoscaler", func(i *contracts.EvaluatePlanInput) { i.Cluster.MIGAutoscalerEnabled = true }, "MIG_AUTOSCALER_ENABLED"},
		{"events not retained", func(i *contracts.EvaluatePlanInput) { i.Cluster.ScalingEventsRetained = false }, "SCALING_EVENTS_NOT_RETAINED"},
	}
}

func withDatabase(i *contracts.EvaluatePlanInput) {
	i.Cluster.Database = contracts.DatabaseScalingPlan{TalksToDatabase: true, PerPodPoolSize: 5, DBConnectionBudget: 100, StorageAutoIncreaseLimit: 500}
}

func databaseCases() []negativeCase {
	return []negativeCase{
		{"db connection ceiling exceeded", func(i *contracts.EvaluatePlanInput) {
			withDatabase(i)
			i.Cluster.Database.DBConnectionBudget = 40
		}, "DB_CONNECTION_CEILING_EXCEEDED"},
		{"unattended prod db scaling without window", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Database.AutomatedVerticalScaleRequested = true
			i.Cluster.Database.OperatorApprovalRecorded = true
		}, "UNATTENDED_PROD_DB_SCALING"},
		{"unattended prod db scaling without approval", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Database.AutomatedVerticalScaleRequested = true
			i.Cluster.Database.InScheduledMaintenanceWindow = true
		}, "UNATTENDED_PROD_DB_SCALING"},
		{"unbounded storage growth", func(i *contracts.EvaluatePlanInput) {
			withDatabase(i)
			i.Cluster.Database.StorageAutoIncreaseLimit = 0
		}, "UNBOUNDED_STORAGE_GROWTH"},
		{"transaction pooling incompatible", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Database.UsesTransactionPooler = true
			i.Cluster.Database.RequiresSessionState = true
		}, "TRANSACTION_POOLING_INCOMPATIBLE"},
	}
}

func ingestionCases() []negativeCase {
	return []negativeCase{
		{"kafka replicas exceed partitions", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Ingestion = contracts.IngestionScalingPlan{Backbone: contracts.BackboneKafka, TopicPartitions: 4, ConsumerMaxReplicas: 8}
		}, "KAFKA_REPLICAS_EXCEED_PARTITIONS"},
		{"pubsub scale to zero in prod", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Ingestion = contracts.IngestionScalingPlan{Backbone: contracts.BackbonePubSub, ConsumerMinReplicas: 0, DualMetricConfigured: true}
		}, "PUBSUB_SCALE_TO_ZERO_IN_PROD"},
		{"pubsub single metric", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Ingestion = contracts.IngestionScalingPlan{Backbone: contracts.BackbonePubSub, ConsumerMinReplicas: 1}
		}, "PUBSUB_SINGLE_METRIC_SCALING"},
	}
}

func serverlessCases() []negativeCase {
	return []negativeCase{
		{"http scale to zero in production", func(i *contracts.EvaluatePlanInput) {
			i.Cluster.Serverless = contracts.ServerlessScaling{HTTPScaleToZero: true, BehindInterceptor: true}
		}, "HTTP_SCALE_TO_ZERO_IN_PRODUCTION"},
		{"http scale to zero without interceptor", func(i *contracts.EvaluatePlanInput) {
			i.Plan.Environment = contracts.EnvironmentNonProduction
			i.Cluster.Serverless = contracts.ServerlessScaling{HTTPScaleToZero: true}
		}, "HTTP_SCALE_TO_ZERO_NO_INTERCEPTOR"},
		{"cold start exceeds budget", func(i *contracts.EvaluatePlanInput) {
			i.Plan.ScaleToZero = true
			i.Cluster.Serverless = contracts.ServerlessScaling{ColdStartSeconds: 20, ColdStartBudgetSeconds: 5, ToleratesReconnectStorm: true}
		}, "COLD_START_EXCEEDS_BUDGET"},
		{"scale to zero reconnect storm", func(i *contracts.EvaluatePlanInput) {
			withDatabase(i)
			i.Plan.ScaleToZero = true
		}, "SCALE_TO_ZERO_RECONNECT_STORM"},
		{"surge and canary push peak over quota", func(i *contracts.EvaluatePlanInput) { i.Plan.MaxReplicas = 18 }, "MAX_SCALE_EXCEEDS_POD_QUOTA"},
	}
}

func withKafkaConsumer(i *contracts.EvaluatePlanInput) {
	i.Cluster.Ingestion = contracts.IngestionScalingPlan{
		Backbone: contracts.BackboneKafka, TopicPartitions: 12, ConsumerMaxReplicas: 12, Signal: contracts.IngestSignalLag,
		DeadLetterConfigured: true, LimitToPartitionsWithLag: true, InvalidOffsetDecided: true,
	}
}

func ingestionSignalCases() []negativeCase {
	return []negativeCase{
		{"ack rate signal", func(i *contracts.EvaluatePlanInput) {
			withKafkaConsumer(i)
			i.Cluster.Ingestion.Signal = contracts.IngestSignalAckRate
		}, "INGEST_SIGNAL_INVALID"},
		{"no dead letter", func(i *contracts.EvaluatePlanInput) {
			withKafkaConsumer(i)
			i.Cluster.Ingestion.DeadLetterConfigured = false
		}, "INGEST_NO_DEAD_LETTER"},
		{"not limited to lag", func(i *contracts.EvaluatePlanInput) {
			withKafkaConsumer(i)
			i.Cluster.Ingestion.LimitToPartitionsWithLag = false
		}, "INGEST_NOT_LIMITED_TO_LAG"},
		{"offset behaviour undecided", func(i *contracts.EvaluatePlanInput) {
			withKafkaConsumer(i)
			i.Cluster.Ingestion.InvalidOffsetDecided = false
		}, "INGEST_OFFSET_BEHAVIOR_UNDECIDED"},
	}
}

func allNegativeCases() []negativeCase {
	var all []negativeCase
	all = append(all, writerCases()...)
	all = append(all, tierCases()...)
	all = append(all, ceilingCases()...)
	all = append(all, spotCases()...)
	all = append(all, operationsCases()...)
	all = append(all, databaseCases()...)
	all = append(all, ingestionCases()...)
	all = append(all, serverlessCases()...)
	all = append(all, ingestionSignalCases()...)
	return all
}
