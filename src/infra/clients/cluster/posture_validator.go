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
		remediation = RemediationPrivateCluster
	}

	return PostureCheckResult{
		ControlID:   ControlIDPrivateCluster,
		Description: DescPrivateCluster,
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
		remediation = RemediationShieldedNodes
	}

	return PostureCheckResult{
		ControlID:   ControlIDShieldedNodes,
		Description: DescShieldedNodes,
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateWorkloadIdentityControl(spec GKEClusterSpec) PostureCheckResult {
	expectedPool := fmt.Sprintf("%s%s", spec.ProjectID, WorkloadPoolSuffix)
	passed := spec.WorkloadPool == expectedPool && spec.ProjectID != ""

	remediation := ""
	if !passed {
		remediation = RemediationWorkloadIdentity
	}

	return PostureCheckResult{
		ControlID:   ControlIDWorkloadIdentity,
		Description: DescWorkloadIdentity,
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateDatabaseEncryptionControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.DatabaseEncryption.State == DatabaseEncryptionStateEncrypted && spec.DatabaseEncryption.KeyName != ""

	remediation := ""
	if !passed {
		remediation = RemediationDatabaseEncryption
	}

	return PostureCheckResult{
		ControlID:   ControlIDDatabaseEncryption,
		Description: DescDatabaseEncryption,
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateBinaryAuthorizationControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.BinaryAuthorization.EvaluationMode == BinaryAuthModeEnforce

	remediation := ""
	if !passed {
		remediation = RemediationBinaryAuthorization
	}

	return PostureCheckResult{
		ControlID:   ControlIDBinaryAuthorization,
		Description: DescBinaryAuthorization,
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateBackupAgentControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.BackupAgentEnabled

	remediation := ""
	if !passed {
		remediation = RemediationBackupAgent
	}

	return PostureCheckResult{
		ControlID:   ControlIDBackupAgent,
		Description: DescBackupAgent,
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
		remediation = RemediationReleaseChannel
	}

	return PostureCheckResult{
		ControlID:   ControlIDReleaseChannel,
		Description: DescReleaseChannel,
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateServiceMeshControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.CloudServiceMeshMode == MeshModeManaged

	remediation := ""
	if !passed {
		remediation = RemediationServiceMesh
	}

	return PostureCheckResult{
		ControlID:   ControlIDServiceMesh,
		Description: DescServiceMesh,
		Passed:      passed,
		Remediation: remediation,
	}
}

func evaluateAuditLoggingControl(spec GKEClusterSpec) PostureCheckResult {
	passed := spec.DataAccessLogsActive

	remediation := ""
	if !passed {
		remediation = RemediationAuditLogging
	}

	return PostureCheckResult{
		ControlID:   ControlIDAuditLogging,
		Description: DescAuditLogging,
		Passed:      passed,
		Remediation: remediation,
	}
}
