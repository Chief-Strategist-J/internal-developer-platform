/*
ALGORITHM & ARCHITECTURE BLUEPRINT: ORCHESTRATOR PORT INTERFACE (V.3 role 06, Invariant A, B)

1. Purpose:
   - Driven port abstracting workload replica scaling and rollout revisions (Argo Rollouts / Kubernetes API).
2. Invariants:
   - Zero vendor-specific types in signature; zero inline comments below header.
*/

package ports

import (
	"context"
	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
)

type OrchestratorPort interface {
	GetCanaryHealth(ctx context.Context, tenantID, workloadID string) (contracts.PodHealthReport, error)
	ScaleCanaryReplicas(ctx context.Context, tenantID, workloadID string, replicas int) error
	PromoteStableRevision(ctx context.Context, tenantID, workloadID, targetImage string) error
}
