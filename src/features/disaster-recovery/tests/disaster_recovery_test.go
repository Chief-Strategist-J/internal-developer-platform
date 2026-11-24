// Package tests provides unit and integration tests for Disaster Recovery domain logic and workflows.
//
// Architectural Blueprint:
// - Step 14 Tests: Validates B-1, B-2 floors, state machine transitions, two-person declaration,
//   fencing epoch increments, and end-to-end failover execution.
// - 100% comment-free function bodies.
package tests

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/features/disaster-recovery/contracts"
	"internal-developer-platform/src/features/disaster-recovery/domain/machines"
	"internal-developer-platform/src/features/disaster-recovery/domain/rules"
	"internal-developer-platform/src/features/disaster-recovery/infrastructure/adapters"
	"internal-developer-platform/src/features/disaster-recovery/service"
)

func TestDRPlanValidationAndFloors(t *testing.T) {
	cmdValid := contracts.CreateDRPlanCommand{
		TenantID:         "tenant-alpha",
		PlanName:         "core-payments-dr",
		IncidentTier:     "TIER_0",
		PrimaryRegion:    "us-central1",
		DRRegion:         "us-east4",
		Topology:         contracts.TopologyWarmStandby,
		TargetRTOSeconds: 300,
		TargetRPOSeconds: 0,
		DataStoreType:    contracts.DataStoreCloudSQL,
		SecretSyncSource: contracts.SecretSourceGoogleSecretManager,
		PrimaryClusterID: "gke-primary-us-central1",
		DRClusterID:      "gke-dr-us-east4",
		IdempotencyKey:   "idem-dr-001",
	}
	if err := contracts.ValidateCreateDRPlanCommand(cmdValid); err != nil {
		t.Fatalf("expected valid command, got: %v", err)
	}

	cmdSameRegion := cmdValid
	cmdSameRegion.DRRegion = "us-central1"
	if err := contracts.ValidateCreateDRPlanCommand(cmdSameRegion); err == nil {
		t.Fatalf("expected error for same region under B-1, got nil")
	}

	cmdViolateRPO := cmdValid
	cmdViolateRPO.TargetRPOSeconds = 30
	if err := contracts.ValidateCreateDRPlanCommand(cmdViolateRPO); err == nil {
		t.Fatalf("expected error for Tier 0 RPO > 0, got nil")
	}

	cmdViolateRTO := cmdValid
	cmdViolateRTO.TargetRTOSeconds = 600
	if err := contracts.ValidateCreateDRPlanCommand(cmdViolateRTO); err == nil {
		t.Fatalf("expected error for Tier 0 RTO > 300s floor, got nil")
	}
}

func TestDRStateMachineTransitions(t *testing.T) {
	st, err := machines.TransitionDRState(contracts.StatusNotReady, machines.EventMarkReady)
	if err != nil || st != contracts.StatusReady {
		t.Fatalf("expected READY, got %s, err: %v", st, err)
	}

	st, err = machines.TransitionDRState(st, machines.EventDeclare)
	if err != nil || st != contracts.StatusDeclared {
		t.Fatalf("expected DECLARED, got %s, err: %v", st, err)
	}

	st, err = machines.TransitionDRState(st, machines.EventStartFencing)
	if err != nil || st != contracts.StatusFencing {
		t.Fatalf("expected FENCING, got %s, err: %v", st, err)
	}

	st, err = machines.TransitionDRState(st, machines.EventStartPromoting)
	if err != nil || st != contracts.StatusPromoting {
		t.Fatalf("expected PROMOTING, got %s, err: %v", st, err)
	}

	if machines.CanAbort(st) {
		t.Fatalf("abort must be forbidden once PROMOTING starts")
	}

	_, err = machines.TransitionDRState(st, machines.EventAbort)
	if err == nil {
		t.Fatalf("expected error aborting at PROMOTING state")
	}
}

func TestTwoPersonRuleEnforcement(t *testing.T) {
	err := rules.ValidateTwoPersonDeclaration("alice", "alice")
	if err == nil {
		t.Fatalf("expected conflict for identical declarer and approver")
	}

	err = rules.ValidateTwoPersonDeclaration("alice", "bob")
	if err != nil {
		t.Fatalf("expected success for distinct declarer and approver, got: %v", err)
	}
}

func TestEndToEndDisasterRecoveryWorkflow(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryDRStoreAdapter()
	cloud := adapters.NewCloudProviderDRAdapter()

	svc := service.NewDisasterRecoveryService(store, cloud, cloud, cloud, cloud)

	cmd := contracts.CreateDRPlanCommand{
		TenantID:         "tenant-beta",
		Source:           "console",
		ExternalID:       "ext-plan-1",
		CorrelationID:    "corr-1",
		PlanName:         "ledger-service-dr",
		IncidentTier:     "TIER_0",
		PrimaryRegion:    "us-central1",
		DRRegion:         "us-east4",
		Topology:         contracts.TopologyWarmStandby,
		TargetRTOSeconds: 300,
		TargetRPOSeconds: 0,
		DataStoreType:    contracts.DataStoreCloudSQL,
		SecretSyncSource: contracts.SecretSourceGoogleSecretManager,
		PrimaryClusterID: "gke-primary",
		DRClusterID:      "gke-dr",
		IdempotencyKey:   "idem-ledger-dr",
	}

	plan, err := svc.CreatePlan(ctx, cmd)
	if err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}
	if plan.Status != contracts.StatusNotReady {
		t.Fatalf("expected initial status NOT_READY, got: %s", plan.Status)
	}

	readiness, err := svc.CheckReadiness(ctx, plan.TenantID, plan.ID)
	if err != nil {
		t.Fatalf("failed to check readiness: %v", err)
	}
	if readiness.OverallStatus != contracts.StatusReady {
		t.Fatalf("expected status READY after probe pass, got: %s", readiness.OverallStatus)
	}

	inc, err := svc.DeclareIncident(ctx, contracts.DeclareIncidentCommand{
		TenantID:      plan.TenantID,
		PlanID:        plan.ID,
		Scope:         contracts.ScopeRegion,
		Declarer:      "operator-1",
		EvidenceLinks: []string{"https://monitoring.internal/incident/101"},
	})
	if err != nil {
		t.Fatalf("failed to declare incident: %v", err)
	}

	approvedInc, err := svc.ApproveIncident(ctx, contracts.ApproveIncidentCommand{
		TenantID:   plan.TenantID,
		IncidentID: inc.ID,
		Approver:   "director-2",
	})
	if err != nil {
		t.Fatalf("failed to approve incident: %v", err)
	}
	if approvedInc.State != "DECLARED" {
		t.Fatalf("expected incident state DECLARED, got: %s", approvedInc.State)
	}

	failoverOp, err := svc.InitiateFailover(ctx, contracts.InitiateFailoverCommand{
		TenantID:   plan.TenantID,
		PlanID:     plan.ID,
		IncidentID: inc.ID,
		Mode:       contracts.FailoverModeSwitchover,
		Initiator:  "lead-sre-1",
	})
	if err != nil {
		t.Fatalf("failed to initiate failover: %v", err)
	}

	completedOp, err := svc.ApproveAndExecuteFailover(ctx, contracts.ApproveFailoverCommand{
		TenantID:    plan.TenantID,
		OperationID: failoverOp.ID,
		Approver:    "vp-eng-2",
	})
	if err != nil {
		t.Fatalf("failed to execute failover: %v", err)
	}
	if completedOp.State != contracts.StatusFailedOver {
		t.Fatalf("expected completed failover state FAILED_OVER, got: %s", completedOp.State)
	}

	epoch, err := svc.GetFencingEpoch(ctx, plan.PlanName)
	if err != nil || epoch.Epoch < 1 {
		t.Fatalf("expected fencing epoch >= 1, got epoch: %+v, err: %v", epoch, err)
	}

	route, err := cloud.GetRoutingPolicy(ctx, plan.PlanName)
	if err != nil || route != plan.DRRegion {
		t.Fatalf("expected traffic routed to DR region '%s', got: '%s'", plan.DRRegion, route)
	}

	failbackFailCmd := contracts.ExecuteFailbackCommand{
		TenantID:   plan.TenantID,
		PlanID:     plan.ID,
		Initiator:  "lead-sre-1",
		Approver:   "vp-eng-2",
		Divergence: 5,
		ReverseLag: 0,
	}
	_, errFail := svc.ExecuteFailback(ctx, failbackFailCmd)
	if errFail == nil {
		t.Fatalf("expected failback to be rejected due to unresolved divergence > 0, got success")
	}

	failbackPassCmd := contracts.ExecuteFailbackCommand{
		TenantID:   plan.TenantID,
		PlanID:     plan.ID,
		Initiator:  "lead-sre-1",
		Approver:   "vp-eng-2",
		Divergence: 0,
		ReverseLag: 0,
	}
	failbackOp, errPass := svc.ExecuteFailback(ctx, failbackPassCmd)
	if errPass != nil {
		t.Fatalf("expected failback to succeed with 0 divergence and 0 reverse lag, got: %v", errPass)
	}
	if failbackOp.State != contracts.StatusNormal {
		t.Fatalf("expected failback state NORMAL, got: %s", failbackOp.State)
	}

	routeAfterFailback, err := cloud.GetRoutingPolicy(ctx, plan.PlanName)
	if err != nil || routeAfterFailback != plan.PrimaryRegion {
		t.Fatalf("expected traffic restored to primary region '%s', got: '%s'", plan.PrimaryRegion, routeAfterFailback)
	}

	restoreCmd := contracts.InitiateRestoreCommand{
		TenantID:       plan.TenantID,
		PlanID:         plan.ID,
		BackupID:       "gke-backup-01",
		TargetCluster:  "gke-dr",
		Namespace:      "payments",
		ConflictPolicy: "FAIL_ON_CONFLICT",
		Initiator:      "sre-lead",
	}
	restoreOp, errRestore := svc.InitiateRestore(ctx, restoreCmd)
	if errRestore != nil {
		t.Fatalf("expected restore to succeed via Backup for GKE manifest validation, got: %v", errRestore)
	}
	if !restoreOp.DataValidated {
		t.Fatalf("expected restored data validation true, got false")
	}

	probeCtx, cancelProbe := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelProbe()
	_ = svc.RunReadinessProbeWorker(probeCtx, 5*time.Millisecond)
}

