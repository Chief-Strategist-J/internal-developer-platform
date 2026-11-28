/*
ALGORITHM & ARCHITECTURE BLUEPRINT: TRAFFIC PORT INTERFACE (V.3 role 06, Invariant A, B)

1. Purpose:
   - Driven port abstracting service mesh / proxy traffic split capabilities.
   - Enforces Invariant A & B: Decoupled abstract interface, 1 file per adapter.
2. Invariants:
   - Zero vendor-specific types in signature; zero inline comments below header.
*/

package ports

import (
	"context"
)

type TrafficPort interface {
	SetTrafficWeight(ctx context.Context, tenantID, workloadID string, weightPct int) error
	GetTrafficWeight(ctx context.Context, tenantID, workloadID string) (int, error)
}
