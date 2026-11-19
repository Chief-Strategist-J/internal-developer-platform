package rules

import (
	"sort"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
)

type ResourceManifest struct {
	Kind      string
	Name      string
	Namespace string
	SyncWave  int
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
