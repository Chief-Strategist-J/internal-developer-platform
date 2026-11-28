/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING TYPES & CONTRACTS (V.3 role 04, AS-INV-01..21)

1. Purpose:
   - Declares the value types for Phase 0, Phase 1, and Phase 2 autoscaling guardrails.
   - Evaluates horizontal/vertical scaling, connection ceilings, storage limits, and event-driven consumer bounds.
2. Invariants:
   - Strictly typed Go structs; zero untyped containers, interface{}, or any.
   - 64-bit integers are string-serialized on the wire; tenant identity never travels in the body (P1-2).
   - Every violation carries the invariant ID it enforces so alerts and runbooks can reference it.
   - Zero inline comments below this header.
*/

package contracts

import "context"

type ReplicaWriter string

const (
	ReplicaWriterHPA  ReplicaWriter = "HPA"
	ReplicaWriterKEDA ReplicaWriter = "KEDA"
	ReplicaWriterNone ReplicaWriter = "NONE"
)

type Environment string

const (
	EnvironmentProduction    Environment = "PRODUCTION"
	EnvironmentNonProduction Environment = "NON_PRODUCTION"
)

type VPAMode string

const (
	VPAModeOff               VPAMode = "OFF"
	VPAModeRecommendOnly     VPAMode = "RECOMMEND_ONLY"
	VPAModeInPlaceOrRecreate VPAMode = "IN_PLACE_OR_RECREATE"
)

type SignalState string

const (
	SignalStateAvailable   SignalState = "AVAILABLE"
	SignalStateUnavailable SignalState = "UNAVAILABLE"
)

type ViolationSeverity string

const (
	ViolationSeverityPage   ViolationSeverity = "PAGE"
	ViolationSeverityTicket ViolationSeverity = "TICKET"
)

type WorkloadRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
}

type ContainerRequests struct {
	CPUMilli    int64 `json:"cpuMilli,string"`
	MemoryBytes int64 `json:"memoryBytes,string"`
}

type VerticalScaling struct {
	Mode    VPAMode  `json:"mode"`
	Metrics []string `json:"metrics"`
}

type ScalingPlan struct {
	TenantID               string            `json:"-"`
	Workload               WorkloadRef       `json:"workload"`
	Environment            Environment       `json:"environment"`
	Tier                   string            `json:"tier"`
	ReplicaWriters         []ReplicaWriter   `json:"replicaWriters"`
	ReplicasPinnedInGit    bool              `json:"replicasPinnedInGit"`
	MinReplicas            int32             `json:"minReplicas"`
	MaxReplicas            int32             `json:"maxReplicas"`
	Zones                  int32             `json:"zones"`
	TopologySpreadDeclared bool              `json:"topologySpreadDeclared"`
	ScaleToZero            bool              `json:"scaleToZero"`
	SynchronousAPI         bool              `json:"synchronousApi"`
	HorizontalMetrics      []string          `json:"horizontalMetrics"`
	Requests               ContainerRequests `json:"requests"`
	Vertical               VerticalScaling   `json:"vertical"`
	RolloutSurgePods       int32             `json:"rolloutSurgePods"`
	CanaryPods             int32             `json:"canaryPods"`
}

type TierPolicy struct {
	Tier             string `json:"tier"`
	MinReplicasFloor int32  `json:"minReplicasFloor"`
	MinZones         int32  `json:"minZones"`
	ScaleToZero      bool   `json:"scaleToZero"`
	TopologySpread   bool   `json:"topologySpread"`
	TierCritical     bool   `json:"tierCritical"`
}

type CapacityCeilings struct {
	QuotaCPUMilli   int64 `json:"quotaCpuMilli,string"`
	QuotaPods       int64 `json:"quotaPods,string"`
	NodePoolMaxPods int64 `json:"nodePoolMaxPods,string"`
	IPHeadroomPods  int64 `json:"ipHeadroomPods,string"`
	BudgetMaxPods   int64 `json:"budgetMaxPods,string"`
}

type SignalObservation struct {
	State            SignalState `json:"state"`
	CurrentReplicas  int32       `json:"currentReplicas"`
	ProposedReplicas int32       `json:"proposedReplicas"`
}

type Violation struct {
	Invariant string            `json:"invariant"`
	Code      string            `json:"code"`
	Severity  ViolationSeverity `json:"severity"`
	Message   string            `json:"message"`
}

type Evaluation struct {
	Allowed    bool        `json:"allowed"`
	Violations []Violation `json:"violations"`
}

type SpotPlacement struct {
	OnSpot                    bool  `json:"onSpot"`
	ClassAllowedOnSpot        bool  `json:"classAllowedOnSpot"`
	Stateful                  bool  `json:"stateful"`
	ShutdownSeconds           int32 `json:"shutdownSeconds"`
	EffectiveGraceSeconds     int32 `json:"effectiveGraceSeconds"`
	StatefulBudgetSeconds     int32 `json:"statefulBudgetSeconds"`
	StandardNonGPUPoolPresent bool  `json:"standardNonGpuPoolPresent"`
}

type ReservedCapacity struct {
	Reserved              bool `json:"reserved"`
	FallbackClassesListed bool `json:"fallbackClassesListed"`
}

type IPPlan struct {
	NodePoolMaxNodes int64 `json:"nodePoolMaxNodes,string"`
	PodsPerNodeRange int64 `json:"podsPerNodeRange,string"`
	PodRangeSize     int64 `json:"podRangeSize,string"`
}

type DisruptionPolicy struct {
	AllowedDisruptions   int32  `json:"allowedDisruptions"`
	UnhealthyPodEviction string `json:"unhealthyPodEviction"`
	BarePods             int32  `json:"barePods"`
}

type CanaryScaling struct {
	Enabled             bool `json:"enabled"`
	AnalysisPerRevision bool `json:"analysisPerRevision"`
	DynamicStableScale  bool `json:"dynamicStableScale"`
	TrafficRouting      bool `json:"trafficRouting"`
}

type Budget struct {
	Currency               string `json:"currency"`
	ProjectedMaxSpendMinor int64  `json:"projectedMaxSpendMinor,string"`
	TenantBudgetMinor      int64  `json:"tenantBudgetMinor,string"`
	OverrunApproved        bool   `json:"overrunApproved"`
}

const (
	BackboneKafka  = "KAFKA"
	BackbonePubSub = "PUBSUB"
)

type DatabaseScalingPlan struct {
	TalksToDatabase                 bool  `json:"talksToDatabase"`
	PerPodPoolSize                  int32 `json:"perPodPoolSize"`
	DBConnectionBudget              int32 `json:"dbConnectionBudget"`
	AutomatedVerticalScaleRequested bool  `json:"automatedVerticalScaleRequested"`
	InScheduledMaintenanceWindow    bool  `json:"inScheduledMaintenanceWindow"`
	OperatorApprovalRecorded        bool  `json:"operatorApprovalRecorded"`
	StorageAutoIncreaseLimit        int64 `json:"storageAutoIncreaseLimit,string"`
	UsesTransactionPooler           bool  `json:"usesTransactionPooler"`
	RequiresSessionState            bool  `json:"requiresSessionState"`
}

const (
	IngestSignalBacklog   = "BACKLOG"
	IngestSignalOldestAge = "OLDEST_AGE"
	IngestSignalLag       = "LAG"
	IngestSignalAckRate   = "ACK_RATE"
	IngestSignalCPUOnly   = "CPU_ONLY"
)

type IngestionScalingPlan struct {
	Backbone                 string `json:"backbone"`
	TopicPartitions          int32  `json:"topicPartitions"`
	ConsumerMaxReplicas      int32  `json:"consumerMaxReplicas"`
	ConsumerMinReplicas      int32  `json:"consumerMinReplicas"`
	DualMetricConfigured     bool   `json:"dualMetricConfigured"`
	Signal                   string `json:"signal"`
	DeadLetterConfigured     bool   `json:"deadLetterConfigured"`
	LimitToPartitionsWithLag bool   `json:"limitToPartitionsWithLag"`
	InvalidOffsetDecided     bool   `json:"invalidOffsetDecided"`
}

type ServerlessScaling struct {
	HTTPScaleToZero         bool  `json:"httpScaleToZero"`
	BehindInterceptor       bool  `json:"behindInterceptor"`
	ColdStartSeconds        int32 `json:"coldStartSeconds"`
	ColdStartBudgetSeconds  int32 `json:"coldStartBudgetSeconds"`
	ToleratesReconnectStorm bool  `json:"toleratesReconnectStorm"`
}

type ClusterState struct {
	Spot                  SpotPlacement        `json:"spot"`
	Reservation           ReservedCapacity     `json:"reservation"`
	IP                    IPPlan               `json:"ip"`
	Disruption            DisruptionPolicy     `json:"disruption"`
	Canary                CanaryScaling        `json:"canary"`
	Budget                Budget               `json:"budget"`
	MIGAutoscalerEnabled  bool                 `json:"migAutoscalerEnabled"`
	ScalingEventsRetained bool                 `json:"scalingEventsRetained"`
	Database              DatabaseScalingPlan  `json:"database"`
	Ingestion             IngestionScalingPlan `json:"ingestion"`
	Serverless            ServerlessScaling    `json:"serverless"`
}

type EvaluatePlanInput struct {
	Plan     ScalingPlan       `json:"plan"`
	Policy   TierPolicy        `json:"policy"`
	Ceilings CapacityCeilings  `json:"ceilings"`
	Signal   SignalObservation `json:"signal"`
	Cluster  ClusterState      `json:"cluster"`
}

type AutoscalingServicePort interface {
	EvaluatePlan(ctx context.Context, input EvaluatePlanInput) (Evaluation, error)
}
