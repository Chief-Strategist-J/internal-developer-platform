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

func SortBySyncWave(manifests []ResourceManifest) []ResourceManifest {
	sorted := make([]ResourceManifest, len(manifests))
	copy(sorted, manifests)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SyncWave < sorted[j].SyncWave
	})
	return sorted
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

func CanInitiateSync(current contracts.SyncPhase) bool {
	switch current {
	case contracts.SyncPhaseSyncing:
		return false
	default:
		return true
	}
}

