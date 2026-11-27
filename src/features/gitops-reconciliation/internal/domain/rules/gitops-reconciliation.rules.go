package rules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"internal-developer-platform/src/features/gitops-reconciliation/internal/contracts"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/errors"
)

type ResourceManifest struct {
	Kind        string
	Name        string
	Namespace   string
	SyncWave    int
	Annotations map[string]string
}

type DriftReport struct {
	HasDrift          bool
	ModifiedResources []string
	DeletedResources  []string
	Critical          bool
}

type SyncWaveBatch struct {
	Wave      int
	Resources []ResourceManifest
}

func SortBySyncWave(manifests []ResourceManifest) []ResourceManifest {
	sorted := make([]ResourceManifest, len(manifests))
	copy(sorted, manifests)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SyncWave < sorted[j].SyncWave
	})
	return sorted
}

func GroupManifestsBySyncWave(manifests []ResourceManifest) []SyncWaveBatch {
	if len(manifests) == 0 {
		return make([]SyncWaveBatch, 0)
	}

	ordered := SortBySyncWave(manifests)
	batches := make([]SyncWaveBatch, 0)

	currentWave := ordered[0].SyncWave
	currentBatch := []ResourceManifest{ordered[0]}

	for idx := 1; idx < len(ordered); idx++ {
		item := ordered[idx]
		if item.SyncWave == currentWave {
			currentBatch = append(currentBatch, item)
		} else {
			batches = append(batches, SyncWaveBatch{
				Wave:      currentWave,
				Resources: currentBatch,
			})
			currentWave = item.SyncWave
			currentBatch = []ResourceManifest{item}
		}
	}

	batches = append(batches, SyncWaveBatch{
		Wave:      currentWave,
		Resources: currentBatch,
	})

	return batches
}

func EvaluateDriftSeverity(modifiedCount int, deletedCount int) DriftReport {
	hasDrift := modifiedCount > 0 || deletedCount > 0
	isCritical := deletedCount > 0 || modifiedCount > 5
	return DriftReport{
		HasDrift:          hasDrift,
		Critical:          isCritical,
		ModifiedResources: make([]string, 0),
		DeletedResources:  make([]string, 0),
	}
}

func ShouldTriggerRollback(syncErr error, health contracts.HealthStatus, lastGoodRevision string) bool {
	if lastGoodRevision == "" {
		return false
	}
	if syncErr != nil {
		return true
	}
	if health == contracts.HealthStatusDegraded || health == contracts.HealthStatusMissing {
		return true
	}
	return false
}

func CanInitiateSync(current contracts.SyncPhase) bool {
	switch current {
	case contracts.SyncPhaseSyncing, contracts.SyncPhaseRollingBack:
		return false
	default:
		return true
	}
}

func VerifyWebhookHMAC(payload []byte, signatureHeader string, secretKey string) bool {
	if secretKey == "" {
		return true
	}
	if signatureHeader == "" {
		return false
	}

	sig := strings.TrimPrefix(signatureHeader, "sha256=")
	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write(payload)
	expectedMAC := mac.Sum(nil)

	return hmac.Equal(sigBytes, expectedMAC)
}

func IsRuntimeIgnoredField(resourceKind, fieldPath string) bool {
	if resourceKind == "Deployment" && (fieldPath == "spec.replicas" || strings.HasPrefix(fieldPath, "spec.replicas")) {
		return true
	}
	if strings.Contains(fieldPath, "traffic-split") || strings.Contains(fieldPath, "mesh.route.weight") {
		return true
	}
	if strings.Contains(fieldPath, "sidecar.istio.io") || strings.Contains(fieldPath, "linkerd.io") {
		return true
	}
	return false
}

func ValidateTenantProjectGuardrails(isTenantScope bool, kind string) error {
	if !isTenantScope {
		return nil
	}

	clusterScoped := map[string]bool{
		"CustomResourceDefinition":       true,
		"ClusterRole":                    true,
		"ClusterRoleBinding":             true,
		"ValidatingWebhookConfiguration": true,
		"MutatingWebhookConfiguration":   true,
	}

	if clusterScoped[kind] {
		return errors.NewForbiddenError("tenants are prohibited from creating cluster-scoped object: " + kind)
	}

	blacklistedGuardrails := map[string]bool{
		"ResourceQuota": true,
		"LimitRange":    true,
		"NetworkPolicy": true,
	}

	if blacklistedGuardrails[kind] {
		return errors.NewForbiddenError("tenants are prohibited from modifying platform guardrail object: " + kind)
	}

	return nil
}

func ValidatePruneSafety(kind string, annotations map[string]string) error {
	dataBearing := map[string]bool{
		"PersistentVolumeClaim": true,
		"Secret":                true,
		"StatefulSet":           true,
	}

	if dataBearing[kind] {
		if annotations == nil || annotations["argocd.argoproj.io/deletion-approved"] != "true" {
			return errors.NewConflictError("data-bearing resource " + kind + " cannot be pruned without deletion-approved annotation")
		}
	}
	return nil
}

func ValidateSecretMechanismConfig(cfg contracts.SecretBackendConfig) error {
	if cfg.TenantID == "" {
		return errors.NewValidationError(constants.ErrSecretTenantIDRequired)
	}
	switch cfg.Mechanism {
	case contracts.SecretMechanismSealedSecrets:
		if cfg.EncryptionKeyRef == "" {
			return errors.NewValidationError(constants.ErrSecretEncryptionKeyRequired)
		}
	case contracts.SecretMechanismExternalSecretsOperator:
		if cfg.SecretStoreRef == "" {
			return errors.NewValidationError(constants.ErrSecretStoreRefRequired)
		}
	case contracts.SecretMechanismSecretsStoreCSI:
		if cfg.CSIDriverName == "" {
			return errors.NewValidationError(constants.ErrSecretCSIDriverRequired)
		}
	default:
		return errors.NewValidationError(constants.ErrSecretUnsupportedMechanism)
	}
	return nil
}

func EvaluateSecretInGitOpsPath(
	mechanism contracts.SecretMechanismType,
	hasPlaintextData bool,
	pluginInjectsValues bool,
) contracts.SecretSyncValidationResult {
	if hasPlaintextData {
		return contracts.SecretSyncValidationResult{
			Allowed:       false,
			Mechanism:     mechanism,
			Reason:        constants.MsgPlaintextSecretInGit,
			ViolationCode: constants.ViolationPlaintextSecretInGit,
		}
	}
	if pluginInjectsValues {
		return contracts.SecretSyncValidationResult{
			Allowed:       false,
			Mechanism:     mechanism,
			Reason:        constants.MsgPluginSecretInjection,
			ViolationCode: constants.ViolationPluginSecretInjection,
		}
	}
	return contracts.SecretSyncValidationResult{
		Allowed:   true,
		Mechanism: mechanism,
		Reason:    constants.MsgSecretReferencePermitted,
	}
}

func EvaluateSourceTrust(projectEnforcesSig bool, commitSigned bool, allowedKey bool, roleReviewed bool) error {
	if !projectEnforcesSig {
		return errors.NewForbiddenError(constants.ErrSourceTrustSigNotEnforced)
	}
	if !commitSigned || !allowedKey {
		return errors.NewUnauthorizedError(constants.ErrSourceTrustSigInvalid)
	}
	if !roleReviewed {
		return errors.NewForbiddenError(constants.ErrSourceTrustReviewRequired)
	}
	return nil
}

func EvaluateSyncWindow(allowWindowExists bool, inAllowWindow bool, inDenyWindow bool, isManualSync bool, windowAllowsManual bool) error {
	if inDenyWindow {
		if isManualSync && windowAllowsManual {
			return nil
		}
		return errors.NewConflictError(constants.ErrSyncBlockedDenyWindow)
	}
	if allowWindowExists && !inAllowWindow {
		if isManualSync && windowAllowsManual {
			return nil
		}
		return errors.NewConflictError(constants.ErrSyncBlockedOutsideAllow)
	}
	return nil
}

func ShouldRetryFailedSHA(currentSHA string, lastFailedSHA string) bool {
	if currentSHA == "" || lastFailedSHA == "" {
		return true
	}
	return currentSHA != lastFailedSHA
}

func ValidateSharedResourceOwnership(existingOwnerApp string, requestingApp string) error {
	if existingOwnerApp != "" && existingOwnerApp != requestingApp {
		return errors.NewConflictError(constants.ErrSharedResourcePrefix + existingOwnerApp)
	}
	return nil
}

func EvaluateBreakGlassAutoSyncGuard(emergencyChangePending bool, emergencyCommittedAndSigned bool) error {
	if emergencyChangePending && !emergencyCommittedAndSigned {
		return errors.NewConflictError("auto-sync blocked: emergency break-glass change must be committed and signed in Git before re-enabling")
	}
	return nil
}

func EvaluateFleetRingHealth(ringResults []contracts.FleetTargetResult) error {
	for _, r := range ringResults {
		if r.Error != "" || r.HealthStatus == contracts.HealthStatusDegraded {
			return errors.NewConflictError("fleet ring rollout halted on degraded cluster: " + r.ClusterName)
		}
	}
	return nil
}
