/*
ALGORITHM & ARCHITECTURE BLUEPRINT: ANALYSIS PORT INTERFACE (V.3 role 06, Invariant A, B)

1. Purpose:
   - Driven port abstracting telemetry query engines (Prometheus / Cloud Monitoring).
2. Invariants:
   - Zero vendor-specific types in signature; zero inline comments below header.
*/

package ports

import (
	"context"
	"time"
)

type AnalysisPort interface {
	QueryMetricSamples(ctx context.Context, tenantID, workloadID, metricQuery string, window time.Duration) ([]float64, error)
}
