/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING API TESTS (V.3 role 14)

 1. Role:
    - Verifies cluster provisioning, posture validation, workload identity manifests, and ledger reconciliation endpoints.
*/
package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	restv1 "internal-developer-platform/src/features/self-service-provisioning/internal/api/rest/v1"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/self-service-provisioning/internal/service"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/cluster"
	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

func createTestClusterSpec(id string) cluster.GKEClusterSpec {
	return cluster.GKEClusterSpec{
		ClusterID:      id,
		ProjectID:      "gov-cloud-prod",
		Location:       "us-central1",
		ReleaseChannel: cluster.ReleaseChannelRegular,
		WorkloadPool:   "gov-cloud-prod" + cluster.WorkloadPoolSuffix,
		PrivateCluster: cluster.PrivateClusterConfig{
			EnablePrivateNodes:    true,
			EnablePrivateEndpoint: true,
			MasterIPv4CIDRBlock:   "172.16.0.0/28",
		},
		ShieldedNodes: cluster.ShieldedNodesConfig{
			Enabled:             true,
			SecureBoot:          true,
			IntegrityMonitoring: true,
		},
		DatabaseEncryption: cluster.DatabaseEncryptionConfig{
			State:   cluster.DatabaseEncryptionStateEncrypted,
			KeyName: "projects/gov-cloud-prod/locations/us-central1/keyRings/ring/cryptoKeys/key",
		},
		BinaryAuthorization: cluster.BinaryAuthorizationConfig{
			EvaluationMode: cluster.BinaryAuthModeEnforce,
		},
		BackupAgentEnabled:   true,
		CloudServiceMeshMode: cluster.MeshModeManaged,
		DataAccessLogsActive: true,
	}
}

func newTestHandler() *restv1.SelfServiceProvisioningHandler {
	resCfg := resilience.DefaultResilienceConfig()
	clAdapter := adapters.NewGKEClusterClientAdapter(cluster.NewGKEClusterClient(resCfg))
	crAdapter := adapters.NewCloudResourceClientAdapter(cloudresource.NewCloudResourceClient(resCfg))
	svc := service.NewSelfServiceProvisioningService(clAdapter, crAdapter)
	return restv1.NewSelfServiceProvisioningHandler(svc)
}

func TestSelfServiceCluster_ProvisionAndValidatePosture(t *testing.T) {
	handler := newTestHandler()
	spec := createTestClusterSpec("cluster-test-01")
	body, _ := json.Marshal(spec)

	provReq := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewReader(body))
	provRec := httptest.NewRecorder()
	handler.ProvisionCluster(provRec, provReq)

	if provRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got: %d", provRec.Code)
	}

	postureReq := httptest.NewRequest(http.MethodGet, constants.EndpointClustersPosture+"?clusterId=cluster-test-01", nil)
	postureRec := httptest.NewRecorder()
	handler.ValidatePosture(postureRec, postureReq)

	if postureRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got: %d", postureRec.Code)
	}

	var resp types.ApiResponse
	if err := json.NewDecoder(postureRec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response")
	}
}

func TestSelfServiceCloudResource_ReconcileLedger(t *testing.T) {
	handler := newTestHandler()
	spec := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:     "app@gov-cloud.iam.gserviceaccount.com",
		KSAName:      "app-sa",
		Namespace:    "app-ns",
		ProjectID:    "gov-cloud",
		RoleBindings: []string{"roles/storage.objectViewer"},
		Metadata: cloudresource.ResourceMetadata{
			RequestID:   "req-1",
			Owner:       "team@gov.io",
			CostCenter:  "CC-1",
			WorkloadID:  "app-workload",
			Environment: "production",
		},
	}
	body, _ := json.Marshal(spec)

	provReq := httptest.NewRequest(http.MethodPost, constants.EndpointCloudResources, bytes.NewReader(body))
	provRec := httptest.NewRecorder()
	handler.ProvisionResource(provRec, provReq)

	if provRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got: %d", provRec.Code)
	}

	reconBody, _ := json.Marshal(map[string][]string{
		"activeWorkloadIds": {"other-workload"},
	})
	reconReq := httptest.NewRequest(http.MethodPost, constants.EndpointCloudResourcesReconcile, bytes.NewReader(reconBody))
	reconRec := httptest.NewRecorder()
	handler.ReconcileLedger(reconRec, reconReq)

	if reconRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got: %d", reconRec.Code)
	}

	var resp types.ApiResponse
	if err := json.NewDecoder(reconRec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response")
	}
}
