package cluster

import (
	"context"
	"sync"
	"time"

	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/shared/errors"
)

type GKEClusterClient struct {
	mu       sync.RWMutex
	clusters map[string]GKEClusterRecord
	executor *resilience.ResilienceExecutor
}

func NewGKEClusterClient(resConfig resilience.ResilienceConfig) *GKEClusterClient {
	return &GKEClusterClient{
		clusters: make(map[string]GKEClusterRecord),
		executor: resilience.NewResilienceExecutor(resConfig),
	}
}

func (c *GKEClusterClient) ProvisionCluster(ctx context.Context, spec GKEClusterSpec) (*GKEClusterRecord, error) {
	if spec.ClusterID == "" || spec.ProjectID == "" {
		return nil, errors.NewValidationError("clusterId and projectId are required")
	}

	var record GKEClusterRecord
	execErr := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		posture := ValidateClusterPosture(spec)

		status := ClusterStatusReady
		if !posture.Compliant {
			status = ClusterStatusNonCompliant
		}

		now := time.Now().UTC()
		record = GKEClusterRecord{
			Spec:      spec,
			Status:    status,
			Posture:   posture,
			Version:   1,
			CreatedAt: now,
			UpdatedAt: now,
		}

		c.mu.Lock()
		c.clusters[spec.ClusterID] = record
		c.mu.Unlock()

		return nil
	})

	if execErr != nil {
		return nil, execErr
	}

	return &record, nil
}

func (c *GKEClusterClient) GetCluster(ctx context.Context, clusterID string) (*GKEClusterRecord, error) {
	if clusterID == "" {
		return nil, errors.NewValidationError("clusterId is required")
	}

	var record GKEClusterRecord
	execErr := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		c.mu.RLock()
		rec, exists := c.clusters[clusterID]
		c.mu.RUnlock()

		if !exists {
			return errors.NewNotFoundError("cluster not found: " + clusterID)
		}

		record = rec
		return nil
	})

	if execErr != nil {
		return nil, execErr
	}

	return &record, nil
}

func (c *GKEClusterClient) ValidatePosture(ctx context.Context, clusterID string) (*ClusterPostureReport, error) {
	cluster, err := c.GetCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	var report ClusterPostureReport
	execErr := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		report = ValidateClusterPosture(cluster.Spec)
		return nil
	})

	if execErr != nil {
		return nil, execErr
	}

	return &report, nil
}

func (c *GKEClusterClient) Resilience() *resilience.ResilienceExecutor {
	return c.executor
}
