package cloudresource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/shared/errors"
)

type CloudResourceClient struct {
	mu       sync.RWMutex
	ledger   map[string]ResourceLedgerRecord
	executor *resilience.ResilienceExecutor
}

func NewCloudResourceClient(resConfig resilience.ResilienceConfig) *CloudResourceClient {
	return &CloudResourceClient{
		ledger:   make(map[string]ResourceLedgerRecord),
		executor: resilience.NewResilienceExecutor(resConfig),
	}
}

func (c *CloudResourceClient) ProvisionWorkloadIdentity(ctx context.Context, spec WorkloadIdentityBindingSpec) (*ManifestResult, error) {
	manifest, err := GenerateWorkloadIdentityManifest(spec)
	if err != nil {
		return nil, err
	}

	resourceID := fmt.Sprintf("wi-%s-%s", spec.Namespace, spec.KSAName)
	now := time.Now().UTC()

	var result *ManifestResult
	execErr := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		c.mu.Lock()
		c.ledger[resourceID] = ResourceLedgerRecord{
			ResourceID:         resourceID,
			ResourceType:       ResourceTypeWorkloadIdentity,
			WorkloadID:         spec.Metadata.WorkloadID,
			Status:             ResourceStatusActive,
			DeletionProtection: false,
			Metadata:           spec.Metadata,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		c.mu.Unlock()

		result = &ManifestResult{
			ResourceID:   resourceID,
			ResourceType: ResourceTypeWorkloadIdentity,
			ManifestYAML: manifest,
			Metadata:     spec.Metadata,
			CreatedAt:    now,
		}
		return nil
	})

	if execErr != nil {
		return nil, execErr
	}
	return result, nil
}

func (c *CloudResourceClient) ProvisionDNSRecord(ctx context.Context, spec DNSRecordSpec) (*ManifestResult, error) {
	manifest, err := GenerateDNSRecordManifest(spec)
	if err != nil {
		return nil, err
	}

	sanitizedName := stringsReplaceAll(stringsTrimSuffix(spec.RecordName, "."), ".", "-")
	resourceID := fmt.Sprintf("dns-%s-%s", spec.ZoneName, sanitizedName)
	now := time.Now().UTC()

	var result *ManifestResult
	execErr := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		c.mu.Lock()
		c.ledger[resourceID] = ResourceLedgerRecord{
			ResourceID:         resourceID,
			ResourceType:       ResourceTypeDNSRecord,
			WorkloadID:         spec.Metadata.WorkloadID,
			Status:             ResourceStatusActive,
			DeletionProtection: false,
			Metadata:           spec.Metadata,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		c.mu.Unlock()

		result = &ManifestResult{
			ResourceID:   resourceID,
			ResourceType: ResourceTypeDNSRecord,
			ManifestYAML: manifest,
			Metadata:     spec.Metadata,
			CreatedAt:    now,
		}
		return nil
	})

	if execErr != nil {
		return nil, execErr
	}
	return result, nil
}

func (c *CloudResourceClient) ProvisionCloudSQLBinding(ctx context.Context, spec CloudSQLBindingSpec) (*ManifestResult, error) {
	manifest, err := GenerateCloudSQLBindingManifest(spec)
	if err != nil {
		return nil, err
	}

	resourceID := fmt.Sprintf("sql-%s-%s", spec.InstanceID, spec.DatabaseName)
	now := time.Now().UTC()

	var result *ManifestResult
	execErr := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		c.mu.Lock()
		c.ledger[resourceID] = ResourceLedgerRecord{
			ResourceID:         resourceID,
			ResourceType:       ResourceTypeCloudSQLBinding,
			WorkloadID:         spec.Metadata.WorkloadID,
			Status:             ResourceStatusActive,
			DeletionProtection: spec.DeletionProtection,
			Metadata:           spec.Metadata,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		c.mu.Unlock()

		result = &ManifestResult{
			ResourceID:   resourceID,
			ResourceType: ResourceTypeCloudSQLBinding,
			ManifestYAML: manifest,
			Metadata:     spec.Metadata,
			CreatedAt:    now,
		}
		return nil
	})

	if execErr != nil {
		return nil, execErr
	}
	return result, nil
}

func (c *CloudResourceClient) TeardownResource(ctx context.Context, resourceID string, force bool) error {
	return c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		c.mu.Lock()
		defer c.mu.Unlock()

		record, exists := c.ledger[resourceID]
		if !exists {
			return errors.NewNotFoundError(fmt.Sprintf(ErrMsgResourceNotFound, resourceID))
		}

		if record.DeletionProtection && !force {
			return errors.NewConflictError(fmt.Sprintf(ErrMsgDeletionProtectionBlocked, resourceID))
		}

		record.Status = ResourceStatusTornDown
		record.UpdatedAt = time.Now().UTC()
		c.ledger[resourceID] = record
		return nil
	})
}

func (c *CloudResourceClient) ReconcileLedger(ctx context.Context, activeWorkloadIDs []string) (*LedgerReconciliationReport, error) {
	activeMap := make(map[string]bool, len(activeWorkloadIDs))
	for _, id := range activeWorkloadIDs {
		activeMap[id] = true
	}

	var report LedgerReconciliationReport
	execErr := c.executor.Execute(ctx, false, func(opCtx context.Context) error {
		c.mu.Lock()
		defer c.mu.Unlock()

		report.TotalManaged = len(c.ledger)
		for id, record := range c.ledger {
			if record.Status == ResourceStatusTornDown {
				continue
			}

			if !activeMap[record.WorkloadID] {
				record.Status = ResourceStatusOrphaned
				record.UpdatedAt = time.Now().UTC()
				c.ledger[id] = record

				report.OrphanedCount++
				report.OrphanedResourceIDs = append(report.OrphanedResourceIDs, id)
			} else {
				report.ActiveCount++
			}
		}
		return nil
	})

	if execErr != nil {
		return nil, execErr
	}
	return &report, nil
}

func (c *CloudResourceClient) GetResource(ctx context.Context, resourceID string) (*ResourceLedgerRecord, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	record, exists := c.ledger[resourceID]
	if !exists {
		return nil, errors.NewNotFoundError(fmt.Sprintf(ErrMsgResourceNotFound, resourceID))
	}
	return &record, nil
}

func (c *CloudResourceClient) ListResourcesByWorkload(ctx context.Context, workloadID string) ([]ResourceLedgerRecord, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	records := make([]ResourceLedgerRecord, 0)
	for _, rec := range c.ledger {
		if rec.WorkloadID == workloadID {
			records = append(records, rec)
		}
	}
	return records, nil
}

func stringsReplaceAll(s, old, replacement string) string {
	res := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if string(s[i]) == old {
			res = append(res, []byte(replacement)...)
		} else {
			res = append(res, s[i])
		}
	}
	return string(res)
}

func stringsTrimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}
