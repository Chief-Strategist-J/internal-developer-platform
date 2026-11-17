package cluster

import (
	"fmt"
	"time"
)

func ValidateClusterPosture(spec GKEClusterSpec) ClusterPostureReport {
	checks := make([]PostureCheckResult, 0, 9)

	checks = append(checks, evaluatePrivateClusterControl(spec))
	checks = append(checks, evaluateShieldedNodesControl(spec))
	checks = append(checks, evaluateWorkloadIdentityControl(spec))
	checks = append(checks, evaluateDatabaseEncryptionControl(spec))
	checks = append(checks, evaluateBinaryAuthorizationControl(spec))
	checks = append(checks, evaluateBackupAgentControl(spec))
	checks = append(checks, evaluateReleaseChannelControl(spec))
	checks = append(checks, evaluateServiceMeshControl(spec))
	checks = append(checks, evaluateAuditLoggingControl(spec))

	driftCount := 0
	compliant := true
	for _, check := range checks {
		if !check.Passed {
			driftCount++
			compliant = false
		}
	}

	return ClusterPostureReport{
		ClusterID:   spec.ClusterID,
		Compliant:   compliant,
		Checks:      checks,
		DriftCount:  driftCount,
		EvaluatedAt: time.Now().UTC(),
	}
}

func evaluatePrivateClusterControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.PrivateCluster.EnablePrivateNodes &&
		spec.PrivateCluster.EnablePrivateEndpoint &&
		spec.PrivateCluster.MasterIPv4CIDRBlock != ""

	remediation := ""
	if !passed {
		remediation = "Enable private nodes, private endpoint, and configure master IPv4 CIDR block"
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-01",
		Description: "Private Cluster Endpoint and Private Worker Nodes",
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateShieldedNodesControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.ShieldedNodes.Enabled &&
		spec.ShieldedNodes.SecureBoot &&
		spec.ShieldedNodes.IntegrityMonitoring

	remediation := ""
	if !passed {
		remediation = "Enable Shielded GKE Nodes with Secure Boot and Integrity Monitoring"
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-02",
		Description: "Shielded GKE Nodes with Secure Boot & Integrity Monitoring",
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateWorkloadIdentityControl(spec GKEClusterSpec) PostureCheckResult {
	expectedPool := fmt.Sprintf("%s.svc.id.goog", spec.ProjectID)
	passed := spec.WorkloadPool == expectedPool && spec.ProjectID != ""

	remediation := ""
	if !passed {
		remediation = fmt.Sprintf("Configure workloadPool to match '%s.svc.id.goog'", spec.ProjectID)
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-03",
		Description: "Workload Identity Federation without Exported Keys",
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateDatabaseEncryptionControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.DatabaseEncryption.State == "ENCRYPTED" && spec.DatabaseEncryption.KeyName != ""

	remediation := ""
	if !passed {
		remediation = "Configure database encryption state to ENCRYPTED with a valid KMS key URI"
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-04",
		Description: "Application-Layer Secrets Encryption via Cloud KMS (CMEK)",
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateBinaryAuthorizationControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.BinaryAuthorization.EvaluationMode == "PROJECT_SINGLETON_POLICY_ENFORCE"

	remediation := ""
	if !passed {
		remediation = "Set binary authorization evaluationMode to PROJECT_SINGLETON_POLICY_ENFORCE"
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-05",
		Description: "Binary Authorization Image Attestation Enforcement",
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateBackupAgentControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.BackupAgentEnabled

	remediation := ""
	if !passed {
		remediation = "Enable Backup for GKE agent on cluster creation"
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-06",
		Description: "Backup for GKE Agent Provisioned",
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateReleaseChannelControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.ReleaseChannel == ReleaseChannelRegular ||
		spec.ReleaseChannel == ReleaseChannelStable ||
		spec.ReleaseChannel == ReleaseChannelRapid

	remediation := ""
	if !passed {
		remediation = "Enroll cluster in a supported release channel (REGULAR, STABLE, RAPID)"
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-07",
		Description: "GKE Release Channel Auto-Upgrade Enrollment",
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateServiceMeshControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.CloudServiceMeshMode == "MANAGED"

	remediation := ""
	if !passed {
		remediation = "Set cloudServiceMeshMode to MANAGED"
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-08",
		Description: "Managed Cloud Service Mesh Injection",
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateAuditLoggingControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.DataAccessLogsActive

	remediation := ""
	if !passed {
		remediation = "Enable Data Access audit logging routed to locked retention storage"
	}

	return PostureCheckResult{
		ControlID:   "GKE-SEC-09",
		Description: "Data Access and System Audit Logging Enabled",
		Passed:      passed,
		Remediation: remediation,
	}
}
