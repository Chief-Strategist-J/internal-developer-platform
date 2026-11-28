/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING RULES TEST SUITE (V.3 role 14, AS-01..AS-15)

1. Purpose:
   - One negative test per violation code, the compliant plan, positive edge cases, and schema/service rejection.
2. Invariants:
   - Pure tests; no mocks, no IO. Zero inline comments below this header.
*/

package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"internal-developer-platform/src/features/autoscaling/internal/contracts"
	"internal-developer-platform/src/features/autoscaling/internal/domain/rules"
	"internal-developer-platform/src/features/autoscaling/internal/service"
)

func hasCode(evaluation contracts.Evaluation, code string) bool {
	for _, v := range evaluation.Violations {
		if v.Code == code {
			return true
		}
	}
	return false
}

func TestEvaluateAll_CompliantPlanAllowed(t *testing.T) {
	evaluation := rules.EvaluateAll(compliantInput())
	if !evaluation.Allowed || len(evaluation.Violations) != 0 {
		t.Fatalf("expected compliant plan to be allowed, got %+v", evaluation.Violations)
	}
}

func TestEvaluateAll_CompliantPlanSerializesEmptyViolationsArray(t *testing.T) {
	raw, err := json.Marshal(rules.EvaluateAll(compliantInput()))
	if err != nil || !strings.Contains(string(raw), `"violations":[]`) {
		t.Fatalf("expected empty violations array, got %s (err=%v)", raw, err)
	}
}

func TestEvaluateAll_NegativeCases(t *testing.T) {
	for _, tt := range allNegativeCases() {
		t.Run(tt.name, func(t *testing.T) {
			input := compliantInput()
			tt.mutate(&input)
			evaluation := rules.EvaluateAll(input)
			if evaluation.Allowed || !hasCode(evaluation, tt.code) {
				t.Fatalf("expected violation %s, got allowed=%v %+v", tt.code, evaluation.Allowed, evaluation.Violations)
			}
		})
	}
}

func TestEvaluateTierFloor_FloorsApplyOnlyInProduction(t *testing.T) {
	input := compliantInput()
	input.Plan.Environment = contracts.EnvironmentNonProduction
	input.Plan.MinReplicas = 1
	input.Plan.Zones = 1
	if got := rules.EvaluateTierFloor(input.Plan, input.Policy); len(got) != 0 {
		t.Fatalf("expected no floor violations outside production, got %+v", got)
	}
}

func TestEvaluateIngestionSignal_CompliantKafkaConsumerAllowed(t *testing.T) {
	input := compliantInput()
	withKafkaConsumer(&input)
	if got := rules.EvaluateIngestionSignal(input.Plan, input.Cluster.Ingestion); len(got) != 0 {
		t.Fatalf("expected compliant consumer to be allowed, got %+v", got)
	}
}

func TestEvaluateScaleToZeroSafety_QueueWorkerBehindBudgetAllowed(t *testing.T) {
	input := compliantInput()
	input.Plan.ScaleToZero = true
	serverless := contracts.ServerlessScaling{ColdStartSeconds: 3, ColdStartBudgetSeconds: 5}
	if got := rules.EvaluateScaleToZeroSafety(input.Plan, input.Cluster.Database, serverless); len(got) != 0 {
		t.Fatalf("expected within-budget scale-to-zero to be allowed, got %+v", got)
	}
}

func TestEvaluateSignalNotIdle_UnavailableScaleUpAllowed(t *testing.T) {
	signal := contracts.SignalObservation{State: contracts.SignalStateUnavailable, CurrentReplicas: 4, ProposedReplicas: 6}
	if got := rules.EvaluateSignalNotIdle(signal); len(got) != 0 {
		t.Fatalf("expected scale-up during signal outage to be allowed, got %+v", got)
	}
}

func TestEvaluateBudget_ApprovedOverrunAllowed(t *testing.T) {
	budget := contracts.Budget{Currency: "USD", ProjectedMaxSpendMinor: 3000, TenantBudgetMinor: 2000, OverrunApproved: true}
	if got := rules.EvaluateBudget(budget); len(got) != 0 {
		t.Fatalf("expected approved overrun to be allowed, got %+v", got)
	}
}

func TestEvaluateSpotPlacement_TolerantClassOnSpotAllowed(t *testing.T) {
	policy := contracts.TierPolicy{Tier: "bronze"}
	spot := contracts.SpotPlacement{OnSpot: true, ClassAllowedOnSpot: true, StandardNonGPUPoolPresent: true, ShutdownSeconds: 10, EffectiveGraceSeconds: 15}
	if got := rules.EvaluateSpotPlacement(policy, spot); len(got) != 0 {
		t.Fatalf("expected tolerant class on spot to be allowed, got %+v", got)
	}
}

func TestSchema_RejectsMalformedInput(t *testing.T) {
	mutations := map[string]func(*contracts.EvaluatePlanInput){
		"empty tenant":      func(i *contracts.EvaluatePlanInput) { i.Plan.TenantID = " " },
		"tier mismatch":     func(i *contracts.EvaluatePlanInput) { i.Plan.Tier = "silver" },
		"unknown env":       func(i *contracts.EvaluatePlanInput) { i.Plan.Environment = "STAGING" },
		"inverted bounds":   func(i *contracts.EvaluatePlanInput) { i.Plan.MinReplicas = 20 },
		"unknown writer":    func(i *contracts.EvaluatePlanInput) { i.Plan.ReplicaWriters = []contracts.ReplicaWriter{"CRON"} },
		"unknown vpa mode":  func(i *contracts.EvaluatePlanInput) { i.Plan.Vertical.Mode = "AUTO" },
		"unknown signal":    func(i *contracts.EvaluatePlanInput) { i.Signal.State = "STALE" },
		"negative quantity": func(i *contracts.EvaluatePlanInput) { i.Ceilings.QuotaPods = -1 },
		"bad currency":      func(i *contracts.EvaluatePlanInput) { i.Cluster.Budget.Currency = "US" },
		"empty workload":    func(i *contracts.EvaluatePlanInput) { i.Plan.Workload.Name = "" },
		"negative cluster":  func(i *contracts.EvaluatePlanInput) { i.Cluster.Disruption.BarePods = -1 },
	}
	svc := service.NewAutoscalingService()
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			input := compliantInput()
			mutate(&input)
			if _, err := svc.EvaluatePlan(context.Background(), input); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestService_EvaluatePlan_ReturnsEvaluation(t *testing.T) {
	svc := service.NewAutoscalingService()
	evaluation, err := svc.EvaluatePlan(context.Background(), compliantInput())
	if err != nil || !evaluation.Allowed {
		t.Fatalf("expected allowed evaluation, got err=%v eval=%+v", err, evaluation)
	}
}

func TestService_EvaluatePlan_HonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.NewAutoscalingService().EvaluatePlan(ctx, compliantInput()); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestEvaluateKafkaPartitionCapping_WithinPartitionsAllowed(t *testing.T) {
	ingest := contracts.IngestionScalingPlan{Backbone: contracts.BackboneKafka, TopicPartitions: 12, ConsumerMaxReplicas: 12}
	if got := rules.EvaluateKafkaPartitionCapping(ingest); len(got) != 0 {
		t.Fatalf("expected consumer within partitions to be allowed, got %+v", got)
	}
}

func TestEvaluatePubSubConsumerFloor_CompliantConsumerAllowed(t *testing.T) {
	plan := compliantPlan()
	ingest := contracts.IngestionScalingPlan{Backbone: contracts.BackbonePubSub, ConsumerMinReplicas: 1, DualMetricConfigured: true}
	if got := rules.EvaluatePubSubConsumerFloor(plan, ingest); len(got) != 0 {
		t.Fatalf("expected compliant consumer to be allowed, got %+v", got)
	}
}

func TestEvaluateNoAutomatedDatabaseScaling_ApprovedWindowAllowed(t *testing.T) {
	db := contracts.DatabaseScalingPlan{AutomatedVerticalScaleRequested: true, InScheduledMaintenanceWindow: true, OperatorApprovalRecorded: true}
	if got := rules.EvaluateNoAutomatedDatabaseScaling(compliantPlan(), db); len(got) != 0 {
		t.Fatalf("expected approved windowed change to be allowed, got %+v", got)
	}
}

func TestEvaluateDatabaseConnectionCeiling_WithinBudgetAllowed(t *testing.T) {
	input := compliantInput()
	withDatabase(&input)
	if got := rules.EvaluateDatabaseConnectionCeiling(input.Plan, input.Cluster.Database); len(got) != 0 {
		t.Fatalf("expected within-budget plan to be allowed, got %+v", got)
	}
}
