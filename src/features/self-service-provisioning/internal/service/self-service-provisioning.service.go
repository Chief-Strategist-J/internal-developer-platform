/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING SERVICE (V.3 role 06)

 1. Role:
    - Stateless orchestrator delegating cluster and cloud resource offerings to injected ports.
 2. Invariants:
    - Zero mutable state beyond injected dependencies (F-02); zero goroutines or tickers (G-11).
    - Zero inline comments below this header.
*/
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
)

type SelfServiceProvisioningService struct {
	clusterPort       ports.ClusterProvisionerPort
	cloudResourcePort ports.CloudResourceProvisionerPort
	ledgerPort        ports.ProvisioningLedgerPort
}

func NewSelfServiceProvisioningService(clusterPort ports.ClusterProvisionerPort, cloudResourcePort ports.CloudResourceProvisionerPort) *SelfServiceProvisioningService {
	return &SelfServiceProvisioningService{
		clusterPort:       clusterPort,
		cloudResourcePort: cloudResourcePort,
	}
}

func (s *SelfServiceProvisioningService) SetLedgerPort(port ports.ProvisioningLedgerPort) {
	s.ledgerPort = port
}

func (s *SelfServiceProvisioningService) ProvisionCluster(ctx context.Context, spec contracts.ClusterSpec) (*contracts.ClusterRecord, error) {
	return observability.RunTraced(ctx, "self_service.service.provision_cluster", func(ctx context.Context) (*contracts.ClusterRecord, error) {
		logger := observability.GetLogger()
		if s.clusterPort == nil {
			err := fmt.Errorf("cluster provisioner port is not configured")
			logger.Error(ctx, "provision cluster failed: port not configured", observability.String("error", err.Error()))
			return nil, err
		}

		intentID := s.recordIntentBeforeEffect(ctx, spec.ProjectID, "CLUSTER", spec.ClusterID)
		rec, err := s.clusterPort.ProvisionCluster(ctx, spec)
		if err != nil {
			s.recordOutcomeAfterEffect(ctx, intentID, "FAILED", "", err.Error())
			logger.Warn(ctx, "provision cluster failed",
				observability.String("cluster_id", spec.ClusterID),
				observability.String("project_id", spec.ProjectID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}

		s.recordOutcomeAfterEffect(ctx, intentID, "READY", spec.ClusterID, "")
		logger.Info(ctx, "provisioned cluster",
			observability.String("cluster_id", spec.ClusterID),
			observability.String("project_id", spec.ProjectID),
		)
		return rec, nil
	}, attribute.String("cluster.id", spec.ClusterID), attribute.String("project.id", spec.ProjectID))
}

func (s *SelfServiceProvisioningService) GetCluster(ctx context.Context, clusterID string) (*contracts.ClusterRecord, error) {
	return observability.RunTraced(ctx, "self_service.service.get_cluster", func(ctx context.Context) (*contracts.ClusterRecord, error) {
		if s.clusterPort == nil {
			err := fmt.Errorf("cluster provisioner port is not configured")
			observability.GetLogger().Error(ctx, "get cluster failed: port not configured", observability.String("error", err.Error()))
			return nil, err
		}
		rec, err := s.clusterPort.GetCluster(ctx, clusterID)
		if err != nil {
			observability.GetLogger().Warn(ctx, "get cluster not found",
				observability.String("cluster_id", clusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		return rec, nil
	}, attribute.String("cluster.id", clusterID))
}

func (s *SelfServiceProvisioningService) ValidateClusterPosture(ctx context.Context, clusterID string) (*contracts.ClusterPostureReport, error) {
	return observability.RunTraced(ctx, "self_service.service.validate_posture", func(ctx context.Context) (*contracts.ClusterPostureReport, error) {
		if s.clusterPort == nil {
			err := fmt.Errorf("cluster provisioner port is not configured")
			observability.GetLogger().Error(ctx, "validate posture failed: port not configured", observability.String("error", err.Error()))
			return nil, err
		}
		report, err := s.clusterPort.ValidatePosture(ctx, clusterID)
		if err != nil {
			observability.GetLogger().Warn(ctx, "validate posture failed",
				observability.String("cluster_id", clusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		return report, nil
	}, attribute.String("cluster.id", clusterID))
}

func (s *SelfServiceProvisioningService) ProvisionWorkloadIdentity(ctx context.Context, spec contracts.WorkloadIdentitySpec) (*contracts.ResourceManifestResult, error) {
	return observability.RunTraced(ctx, "self_service.service.provision_workload_identity", func(ctx context.Context) (*contracts.ResourceManifestResult, error) {
		logger := observability.GetLogger()
		if s.cloudResourcePort == nil {
			err := fmt.Errorf("cloud resource provisioner port is not configured")
			logger.Error(ctx, "provision workload identity failed: port not configured", observability.String("error", err.Error()))
			return nil, err
		}

		targetID := spec.ProjectID + "/" + spec.Namespace + "/" + spec.KSAName
		intentID := s.recordIntentBeforeEffect(ctx, spec.Metadata.Owner, "WORKLOAD_IDENTITY", targetID)
		res, err := s.cloudResourcePort.ProvisionWorkloadIdentity(ctx, spec)
		if err != nil {
			s.recordOutcomeAfterEffect(ctx, intentID, "FAILED", "", err.Error())
			logger.Warn(ctx, "provision workload identity failed",
				observability.String("target_id", targetID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}

		s.recordOutcomeAfterEffect(ctx, intentID, "READY", targetID, "")
		logger.Info(ctx, "provisioned workload identity",
			observability.String("target_id", targetID),
			observability.String("project_id", spec.ProjectID),
		)
		return res, nil
	}, attribute.String("project.id", spec.ProjectID), attribute.String("ksa.name", spec.KSAName))
}

func (s *SelfServiceProvisioningService) ReconcileLedger(ctx context.Context, activeWorkloadIDs []string) (*contracts.LedgerReconciliationReport, error) {
	return observability.RunTraced(ctx, "self_service.service.reconcile_ledger", func(ctx context.Context) (*contracts.LedgerReconciliationReport, error) {
		if s.cloudResourcePort == nil {
			err := fmt.Errorf("cloud resource provisioner port is not configured")
			observability.GetLogger().Error(ctx, "reconcile ledger failed: port not configured", observability.String("error", err.Error()))
			return nil, err
		}
		report, err := s.cloudResourcePort.ReconcileLedger(ctx, activeWorkloadIDs)
		if err != nil {
			observability.GetLogger().Error(ctx, "reconcile ledger failed", observability.String("error", err.Error()))
			return nil, err
		}
		return report, nil
	}, attribute.Int("active_workloads.count", len(activeWorkloadIDs)))
}

func (s *SelfServiceProvisioningService) recordIntentBeforeEffect(ctx context.Context, fallbackTenant string, resourceKind string, targetID string) string {
	if s.ledgerPort == nil || targetID == "" {
		return ""
	}
	tenantID := observability.ExtractTenantID(ctx)
	if tenantID == "" {
		tenantID = fallbackTenant
	}
	if tenantID == "" {
		tenantID = "platform-default"
	}
	intentID := fmt.Sprintf("ssp-%s-%d", resourceKind, time.Now().UnixNano())
	sum := sha256.Sum256([]byte(intentID + ":" + tenantID + ":" + targetID))
	publicID := "sspi_" + hex.EncodeToString(sum[:13])
	_ = s.ledgerPort.RecordIntent(ctx, contracts.ProvisioningIntent{
		ID:           intentID,
		PublicID:     publicID,
		TenantID:     tenantID,
		ResourceKind: resourceKind,
		TargetID:     targetID,
		Attempt:      1,
		Status:       "PENDING",
	})
	return intentID
}

func (s *SelfServiceProvisioningService) recordOutcomeAfterEffect(ctx context.Context, intentID string, status string, outcomeID string, errMsg string) {
	if s.ledgerPort == nil || intentID == "" {
		return
	}
	_ = s.ledgerPort.RecordOutcome(ctx, intentID, 1, status, outcomeID, errMsg)
}
