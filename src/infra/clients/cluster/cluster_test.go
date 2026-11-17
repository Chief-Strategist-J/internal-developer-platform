package cluster

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/infra/clients/resilience"
)

func createHardenedClusterSpec(id string) GKEClusterSpec {
	return GKEClusterSpec{
		ClusterID:      id,
		ProjectID:      "gov-cloud-prod",
		Location:       "us-central1",
		ReleaseChannel: ReleaseChannelRegular,
		WorkloadPool:   "gov-cloud-prod.svc.id.goog",
		PrivateCluster: PrivateClusterConfig{
			EnablePrivateNodes:    true,
			EnablePrivateEndpoint: true,
			MasterIPv4CIDRBlock:   "172.16.0.0/28",
		},
		ShieldedNodes: ShieldedNodesConfig{
			Enabled:             true,
			SecureBoot:          true,
			IntegrityMonitoring: true,
		},
		DatabaseEncryption: DatabaseEncryptionConfig{
			State:   "ENCRYPTED",
			KeyName: "projects/gov-cloud-prod/locations/us-central1/keyRings/ring/cryptoKeys/key",
		},
		BinaryAuthorization: BinaryAuthorizationConfig{
			EvaluationMode: "PROJECT_SINGLETON_POLICY_ENFORCE",
		},
		BackupAgentEnabled:   true,
		CloudServiceMeshMode: "MANAGED",
		DataAccessLogsActive: true,
	}
}

func TestHardenedClusterProvisionSuccess(t *testing.T) {
	client := NewGKEClusterClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := createHardenedClusterSpec("cluster-prod-01")
	record, err := client.ProvisionCluster(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error provisioning cluster: %v", err)
	}

	if record.Status != ClusterStatusReady {
		t.Fatalf("expected status READY, got %s", record.Status)
	}
	if !record.Posture.Compliant {
		t.Fatalf("expected posture to be fully compliant")
	}
	if record.Posture.DriftCount != 0 {
		t.Fatalf("expected drift count 0, got %d", record.Posture.DriftCount)
	}
}

func TestNonCompliantClusterRejection(t *testing.T) {
	client := NewGKEClusterClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := createHardenedClusterSpec("cluster-insecure-01")
	spec.PrivateCluster.EnablePrivateEndpoint = false
	spec.ShieldedNodes.SecureBoot = false
	spec.DatabaseEncryption.State = "DECRYPTED"
	spec.BinaryAuthorization.EvaluationMode = "DISABLED"

	record, err := client.ProvisionCluster(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error provisioning cluster: %v", err)
	}

	if record.Status != ClusterStatusNonCompliant {
		t.Fatalf("expected status NON_COMPLIANT, got %s", record.Status)
	}
	if record.Posture.Compliant {
		t.Fatalf("expected posture to fail compliance check")
	}
	if record.Posture.DriftCount != 4 {
		t.Fatalf("expected 4 drift violations, got %d", record.Posture.DriftCount)
	}
}

func TestClusterPostureValidationQuery(t *testing.T) {
	client := NewGKEClusterClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := createHardenedClusterSpec("cluster-prod-02")
	_, err := client.ProvisionCluster(ctx, spec)
	if err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	report, err := client.ValidatePosture(ctx, "cluster-prod-02")
	if err != nil {
		t.Fatalf("posture check failed: %v", err)
	}

	if !report.Compliant {
		t.Fatalf("expected report compliant")
	}
	if len(report.Checks) != 9 {
		t.Fatalf("expected 9 baseline posture checks, got %d", len(report.Checks))
	}
}

func TestClusterClientDeadlineBudget(t *testing.T) {
	resCfg := resilience.DefaultResilienceConfig()
	resCfg.MinDeadlineMargin = 50 * time.Millisecond
	resCfg.ConnectTimeout = 100 * time.Millisecond

	client := NewGKEClusterClient(resCfg)
	expiredCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	spec := createHardenedClusterSpec("cluster-timeout-01")
	_, err := client.ProvisionCluster(expiredCtx, spec)
	if err == nil {
		t.Fatalf("expected deadline exceeded error")
	}
}
