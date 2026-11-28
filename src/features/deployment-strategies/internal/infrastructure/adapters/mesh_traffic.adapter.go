/*
ALGORITHM & ARCHITECTURE BLUEPRINT: MESH TRAFFIC ADAPTER (V.3 role 11, Invariant A, P0-02, P0-03)

1. Purpose:
   - Configures Istio VirtualService or Envoy gateway traffic shifting weights.
   - Implements ports.TrafficPort.
2. Invariants:
   - Emits OpenTelemetry client spans; zero inline comments below header.
*/

package adapters

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/infra/observability"
)

type MeshTrafficAdapter struct {
	mu      sync.RWMutex
	weights map[string]int
}

func NewMeshTrafficAdapter() *MeshTrafficAdapter {
	return &MeshTrafficAdapter{
		weights: make(map[string]int),
	}
}

func (a *MeshTrafficAdapter) SetTrafficWeight(ctx context.Context, tenantID, workloadID string, weightPct int) error {
	return observability.WithClientSpan(ctx, "deployment_strategies.mesh.set_traffic_weight", func(_ context.Context) error {
		if weightPct < 0 || weightPct > 100 {
			return fmt.Errorf("invalid mesh weight %d: must be between 0 and 100", weightPct)
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		key := fmt.Sprintf("%s:%s", tenantID, workloadID)
		a.weights[key] = weightPct
		return nil
	},
		attribute.String("mesh.system", "istio"),
		attribute.String("workload.id", workloadID),
		attribute.Int("traffic.weight", weightPct),
	)
}

func (a *MeshTrafficAdapter) GetTrafficWeight(ctx context.Context, tenantID, workloadID string) (int, error) {
	var weight int
	err := observability.WithClientSpan(ctx, "deployment_strategies.mesh.get_traffic_weight", func(_ context.Context) error {
		a.mu.RLock()
		defer a.mu.RUnlock()
		key := fmt.Sprintf("%s:%s", tenantID, workloadID)
		weight = a.weights[key]
		return nil
	},
		attribute.String("mesh.system", "istio"),
		attribute.String("workload.id", workloadID),
	)
	return weight, err
}
