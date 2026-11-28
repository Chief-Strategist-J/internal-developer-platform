/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES TEST SUITE (V.3 role 16, P0-01..P0-08)

1. Purpose:
   - Comprehensive test suite verifying FSM transitions, Invariant P0-03 readiness gates, Invariant P0-04 automated aborts, Data Contract v1.2 ledger constraints, and end-to-end progressive canary workflows.
2. Invariants:
   - Zero inline comments below header.
*/

package tests

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
	"internal-developer-platform/src/features/deployment-strategies/internal/domain/machines"
	"internal-developer-platform/src/features/deployment-strategies/internal/domain/rules"
	"internal-developer-platform/src/features/deployment-strategies/internal/domain/workflows"
	"internal-developer-platform/src/features/deployment-strategies/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/deployment-strategies/internal/service"
)

func TestRolloutStateMachine_Transitions(t *testing.T) {
	tests := []struct {
		name    string
		from    contracts.RolloutStatus
		to      contracts.RolloutStatus
		allowed bool
	}{
		{"init to routing step", contracts.RolloutStatusInitializing, contracts.RolloutStatusRoutingStep, true},
		{"init to aborted", contracts.RolloutStatusInitializing, contracts.RolloutStatusAborted, true},
		{"init to completed", contracts.RolloutStatusInitializing, contracts.RolloutStatusCompleted, false},
		{"routing to analyzing", contracts.RolloutStatusRoutingStep, contracts.RolloutStatusAnalyzing, true},
		{"routing to aborting", contracts.RolloutStatusRoutingStep, contracts.RolloutStatusAborting, true},
		{"routing to completed", contracts.RolloutStatusRoutingStep, contracts.RolloutStatusCompleted, false},
		{"analyzing to routing next step", contracts.RolloutStatusAnalyzing, contracts.RolloutStatusRoutingStep, true},
		{"analyzing to promoting", contracts.RolloutStatusAnalyzing, contracts.RolloutStatusPromoting, true},
		{"analyzing to paused", contracts.RolloutStatusAnalyzing, contracts.RolloutStatusPaused, true},
		{"analyzing to aborting", contracts.RolloutStatusAnalyzing, contracts.RolloutStatusAborting, true},
		{"paused to analyzing", contracts.RolloutStatusPaused, contracts.RolloutStatusAnalyzing, true},
		{"paused to aborting", contracts.RolloutStatusPaused, contracts.RolloutStatusAborting, true},
		{"promoting to completed", contracts.RolloutStatusPromoting, contracts.RolloutStatusCompleted, true},
		{"promoting to aborting", contracts.RolloutStatusPromoting, contracts.RolloutStatusAborting, true},
		{"aborting to aborted", contracts.RolloutStatusAborting, contracts.RolloutStatusAborted, true},
		{"completed is terminal", contracts.RolloutStatusCompleted, contracts.RolloutStatusAnalyzing, false},
		{"aborted is terminal", contracts.RolloutStatusAborted, contracts.RolloutStatusRoutingStep, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := machines.ValidateTransition(tt.from, tt.to)
			if tt.allowed && err != nil {
				t.Fatalf("expected transition %s -> %s to be allowed, got error: %v", tt.from, tt.to, err)
			}
			if !tt.allowed && err == nil {
				t.Fatalf("expected transition %s -> %s to be rejected, got nil", tt.from, tt.to)
			}
		})
	}
}

func TestInvariantP03_PrePodTrafficShift(t *testing.T) {
	healthyReport := contracts.PodHealthReport{
		ReplicasDesired: 2,
		ReplicasReady:   2,
		SidecarInjected: true,
		SidecarHealthy:  true,
	}
	if err := rules.ValidateCanaryReadiness(healthyReport); err != nil {
		t.Fatalf("expected healthy report to pass, got: %v", err)
	}

	unreadyReport := contracts.PodHealthReport{
		ReplicasDesired: 2,
		ReplicasReady:   1,
		SidecarInjected: true,
		SidecarHealthy:  true,
	}
	if err := rules.ValidateCanaryReadiness(unreadyReport); err == nil {
		t.Fatal("expected unready replicas to fail Invariant P0-03")
	}

	missingSidecar := contracts.PodHealthReport{
		ReplicasDesired: 2,
		ReplicasReady:   2,
		SidecarInjected: false,
		SidecarHealthy:  false,
	}
	if err := rules.ValidateCanaryReadiness(missingSidecar); err == nil {
		t.Fatal("expected missing sidecar proxy to fail Invariant P0-03")
	}
}

func TestMetricAnalysis_Evaluation(t *testing.T) {
	samplesPass := []float64{0.1, 0.2, 0.15}
	verdict, val, err := rules.EvaluateMetricThreshold(samplesPass, 0.5, 3)
	if err != nil || verdict != contracts.AnalysisVerdictPass {
		t.Fatalf("expected pass, got verdict=%s avg=%.2f err=%v", verdict, val, err)
	}

	samplesFail := []float64{0.8, 0.9, 0.7}
	verdict, val, err = rules.EvaluateMetricThreshold(samplesFail, 0.5, 3)
	if err != nil || verdict != contracts.AnalysisVerdictFail {
		t.Fatalf("expected fail, got verdict=%s avg=%.2f err=%v", verdict, val, err)
	}

	samplesInconclusive := []float64{0.1}
	verdict, _, err = rules.EvaluateMetricThreshold(samplesInconclusive, 0.5, 3)
	if err == nil || verdict != contracts.AnalysisVerdictInconclusive {
		t.Fatalf("expected inconclusive error, got verdict=%s err=%v", verdict, err)
	}
}

func TestExpandContract_BreakingMigration(t *testing.T) {
	if err := rules.ValidateExpandContractMigration("ADD COLUMN email VARCHAR(255)"); err != nil {
		t.Fatalf("expected backward-compatible migration to pass, got: %v", err)
	}
	if err := rules.ValidateExpandContractMigration("ALTER TABLE users DROP COLUMN phone"); err == nil {
		t.Fatal("expected drop column migration to be blocked by Case 04 check")
	}
	if err := rules.ValidateExpandContractMigration("ALTER TABLE orders RENAME COLUMN amt TO total"); err == nil {
		t.Fatal("expected rename column migration to be blocked by Case 04 check")
	}
}

func TestCanaryRolloutWorkflow_EndToEnd(t *testing.T) {
	ctx := context.Background()
	tenantID := "tenant-alpha"
	repo := adapters.NewMemoryRepositoryAdapter()
	traffic := adapters.NewMemoryTrafficAdapter()
	orchestrator := adapters.NewArgoRolloutsAdapter()
	analysis := adapters.NewPrometheusAnalysisAdapter()

	plan := contracts.RolloutPlan{
		ID:               "plan-canary-01",
		PublicID:         "dplan_0123456789abcdef0123456789",
		TenantID:         tenantID,
		Name:             "canary-three-step",
		StrategyType:     contracts.StrategyTypeCanary,
		FailureThreshold: 1.0,
		CorrelationID:    "corr-123",
		Version:          1,
		Steps: []contracts.CanaryStep{
			{StepIndex: 0, WeightPct: 10, PauseDuration: 1 * time.Second},
			{StepIndex: 1, WeightPct: 25, PauseDuration: 1 * time.Second},
			{StepIndex: 2, WeightPct: 50, PauseDuration: 1 * time.Second},
		},
	}
	if err := repo.CreatePlan(ctx, plan); err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}

	analysis.SetMockMetricSamples(tenantID, "workload-payment", "sum(rate(http_requests_total{status=~'5..'}[1m])) / sum(rate(http_requests_total[1m])) * 100", []float64{0.2, 0.1})

	wf := workflows.NewCanaryRolloutWorkflow(traffic, orchestrator, analysis, repo)

	exec := contracts.RolloutExecution{
		ID:               "exec-001",
		PublicID:         "dexec_0123456789abcdef0123456789",
		TenantID:         tenantID,
		PlanID:           plan.ID,
		WorkloadID:       "workload-payment",
		TargetImage:      "gcr.io/payment:v2.0.0",
		Status:           contracts.RolloutStatusInitializing,
		CurrentStepIndex: 0,
		CurrentWeightPct: 0,
		CorrelationID:    "corr-123",
		Version:          1,
	}
	if err := repo.CreateExecution(ctx, exec); err != nil {
		t.Fatalf("failed to create execution: %v", err)
	}

	if err := wf.ExecuteStepProgression(ctx, tenantID, exec.ID); err != nil {
		t.Fatalf("step 0 progression failed: %v", err)
	}
	w0, _ := traffic.GetTrafficWeight(ctx, tenantID, "workload-payment")
	if w0 != 10 {
		t.Fatalf("expected step 0 weight 10, got %d", w0)
	}

	if err := wf.ExecuteStepProgression(ctx, tenantID, exec.ID); err != nil {
		t.Fatalf("step 1 progression failed: %v", err)
	}
	w1, _ := traffic.GetTrafficWeight(ctx, tenantID, "workload-payment")
	if w1 != 25 {
		t.Fatalf("expected step 1 weight 25, got %d", w1)
	}

	if err := wf.ExecuteStepProgression(ctx, tenantID, exec.ID); err != nil {
		t.Fatalf("final step 2 and promotion progression failed: %v", err)
	}

	finalExec, err := repo.GetExecution(ctx, tenantID, exec.ID)
	if err != nil {
		t.Fatalf("failed to get final execution: %v", err)
	}
	if finalExec.Status != contracts.RolloutStatusCompleted {
		t.Fatalf("expected status %s, got %s", contracts.RolloutStatusCompleted, finalExec.Status)
	}
	finalWeight, _ := traffic.GetTrafficWeight(ctx, tenantID, "workload-payment")
	if finalWeight != 0 {
		t.Fatalf("expected canary mesh weight to be zeroed after promotion, got %d", finalWeight)
	}
}

func TestCanaryRolloutWorkflow_AutomatedAbort(t *testing.T) {
	ctx := context.Background()
	tenantID := "tenant-beta"
	repo := adapters.NewMemoryRepositoryAdapter()
	traffic := adapters.NewMemoryTrafficAdapter()
	orchestrator := adapters.NewArgoRolloutsAdapter()
	analysis := adapters.NewPrometheusAnalysisAdapter()

	plan := contracts.RolloutPlan{
		ID:               "plan-canary-abort",
		PublicID:         "dplan_0123456789abcdef012345678a",
		TenantID:         tenantID,
		Name:             "canary-abort-test",
		StrategyType:     contracts.StrategyTypeCanary,
		FailureThreshold: 0.5,
		Version:          1,
		Steps: []contracts.CanaryStep{
			{StepIndex: 0, WeightPct: 20, PauseDuration: 1 * time.Second},
		},
	}
	_ = repo.CreatePlan(ctx, plan)

	analysis.SetMockMetricSamples(tenantID, "workload-order", "sum(rate(http_requests_total{status=~'5..'}[1m])) / sum(rate(http_requests_total[1m])) * 100", []float64{2.5, 3.1})

	svc := service.NewDeploymentStrategiesService(repo, traffic, orchestrator, analysis)
	exec, err := svc.StartRollout(ctx, contracts.StartRolloutInput{
		TenantID:    tenantID,
		PlanID:      plan.ID,
		WorkloadID:  "workload-order",
		TargetImage: "gcr.io/order:v2.1.0-buggy",
	})
	if err != nil {
		t.Fatalf("failed to start rollout: %v", err)
	}
	if exec.Status != contracts.RolloutStatusAborted {
		t.Fatalf("expected status %s due to metric breach, got %s", contracts.RolloutStatusAborted, exec.Status)
	}
	weight, _ := traffic.GetTrafficWeight(ctx, tenantID, "workload-order")
	if weight != 0 {
		t.Fatalf("expected mesh weight 0 after abort, got %d", weight)
	}
}
