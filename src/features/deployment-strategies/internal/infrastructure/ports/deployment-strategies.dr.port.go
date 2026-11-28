/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DISASTER RECOVERY GUARD PORT (V.3 role 06, Invariant P0-12, X-03)

1. Purpose:
   - Outbound port defining cross-region DR fencing and incident checks.
   - Prevents split-brain canary traffic progression during regional failover.
2. Invariants:
   - Zero inline comments below header.
*/

package ports

import "context"

type DRGuardPort interface {
	IsWorkloadFenced(ctx context.Context, tenantID, workloadID string) (bool, error)
}
