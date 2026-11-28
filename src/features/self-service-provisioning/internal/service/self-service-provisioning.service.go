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
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
)

type SelfServiceProvisioningService struct {
	clusterPort       ports.ClusterProvisionerPort
	cloudResourcePort ports.CloudResourceProvisionerPort
	ledgerPort        ports.ProvisioningLedgerPort
	reservationPort   ports.ProvisioningReservationPort
	approvalPort      ports.ProvisioningApprovalPort
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

func (s *SelfServiceProvisioningService) SetReservationPort(port ports.ProvisioningReservationPort) {
	s.reservationPort = port
}

func (s *SelfServiceProvisioningService) SetApprovalPort(port ports.ProvisioningApprovalPort) {
	s.approvalPort = port
}

func (s *SelfServiceProvisioningService) ProvisionCluster(ctx context.Context, spec contracts.ClusterSpec) (*contracts.ClusterRecord, error) {
	return observability.RunTraced(ctx, "self_service.service.provision_cluster", func(ctx context.Context) (*contracts.ClusterRecord, error) {
		logger := observability.GetLogger()
		if s.clusterPort == nil {
			err := fmt.Errorf("cluster provisioner port is not configured")
			logger.Error(ctx, "provision cluster failed: port not configured", observability.String("error", err.Error()))
			return nil, err
		}

		if err := s.verifyApprovalIfPresent(ctx); err != nil {
			logger.Warn(ctx, "provision cluster rejected by approval/plan pinning gate",
				observability.String("cluster_id", spec.ClusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}

		reservationID, err := s.reserveBeforeEffect(ctx, spec.ProjectID, "CLUSTER", spec.ClusterID, 1, 1)
		if err != nil {
			logger.Warn(ctx, "provision cluster rejected by capacity/name reservation",
				observability.String("cluster_id", spec.ClusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}

		intentID, attempt, err := s.recordIntentBeforeEffect(ctx, spec.ProjectID, "CLUSTER", spec.ClusterID)
		if err != nil {
			s.releaseReservation(ctx, reservationID)
			logger.Error(ctx, "provision cluster aborted: intent persistence failed",
				observability.String("cluster_id", spec.ClusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		rec, err := s.clusterPort.ProvisionCluster(ctx, spec)
		if err != nil {
			s.releaseReservation(ctx, reservationID)
			if outcomeErr := s.recordOutcomeAfterEffect(ctx, intentID, attempt, "FAILED", "", err.Error()); outcomeErr != nil {
				logger.Warn(ctx, "provision cluster outcome fenced",
					observability.String("cluster_id", spec.ClusterID),
					observability.String("error", outcomeErr.Error()),
				)
				return nil, outcomeErr
			}
			logger.Warn(ctx, "provision cluster failed",
				observability.String("cluster_id", spec.ClusterID),
				observability.String("project_id", spec.ProjectID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}

		s.commitReservation(ctx, reservationID)

		if err := s.recordOutcomeAfterEffect(ctx, intentID, attempt, "READY", spec.ClusterID, ""); err != nil {
			logger.Warn(ctx, "provision cluster outcome fenced",
				observability.String("cluster_id", spec.ClusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
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
		if err := s.verifyApprovalIfPresent(ctx); err != nil {
			logger.Warn(ctx, "provision workload identity rejected by approval/plan pinning gate",
				observability.String("target_id", targetID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}

		reservationID, err := s.reserveBeforeEffect(ctx, spec.Metadata.Owner, "WORKLOAD_IDENTITY", targetID, 1, 0)
		if err != nil {
			logger.Warn(ctx, "provision workload identity rejected by capacity/name reservation",
				observability.String("target_id", targetID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}

		intentID, attempt, err := s.recordIntentBeforeEffect(ctx, spec.Metadata.Owner, "WORKLOAD_IDENTITY", targetID)
		if err != nil {
			s.releaseReservation(ctx, reservationID)
			logger.Error(ctx, "provision workload identity aborted: intent persistence failed",
				observability.String("target_id", targetID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		res, err := s.cloudResourcePort.ProvisionWorkloadIdentity(ctx, spec)
		if err != nil {
			s.releaseReservation(ctx, reservationID)
			if outcomeErr := s.recordOutcomeAfterEffect(ctx, intentID, attempt, "FAILED", "", err.Error()); outcomeErr != nil {
				logger.Warn(ctx, "provision workload identity outcome fenced",
					observability.String("target_id", targetID),
					observability.String("error", outcomeErr.Error()),
				)
				return nil, outcomeErr
			}
			logger.Warn(ctx, "provision workload identity failed",
				observability.String("target_id", targetID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}

		s.commitReservation(ctx, reservationID)

		if err := s.recordOutcomeAfterEffect(ctx, intentID, attempt, "READY", targetID, ""); err != nil {
			logger.Warn(ctx, "provision workload identity outcome fenced",
				observability.String("target_id", targetID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
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

type attemptContextKey struct{}

func WithProvisionAttempt(ctx context.Context, attempt int) context.Context {
	return context.WithValue(ctx, attemptContextKey{}, attempt)
}

func extractProvisionAttempt(ctx context.Context) int {
	if val, ok := ctx.Value(attemptContextKey{}).(int); ok && val > 0 {
		return val
	}
	return 1
}

func (s *SelfServiceProvisioningService) recordIntentBeforeEffect(ctx context.Context, fallbackTenant string, resourceKind string, targetID string) (string, int, error) {
	attempt := extractProvisionAttempt(ctx)
	if s.ledgerPort == nil || targetID == "" {
		return "", attempt, nil
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
	err := s.ledgerPort.RecordIntent(ctx, contracts.ProvisioningIntent{
		ID:           intentID,
		PublicID:     publicID,
		TenantID:     tenantID,
		ResourceKind: resourceKind,
		TargetID:     targetID,
		Attempt:      attempt,
		Status:       "PENDING",
	})
	if err != nil {
		return "", attempt, err
	}
	return intentID, attempt, nil
}

func (s *SelfServiceProvisioningService) recordOutcomeAfterEffect(ctx context.Context, intentID string, attempt int, status string, outcomeID string, errMsg string) error {
	if s.ledgerPort == nil || intentID == "" {
		return nil
	}
	return s.ledgerPort.RecordOutcome(ctx, intentID, attempt, status, outcomeID, errMsg)
}

func (s *SelfServiceProvisioningService) reserveBeforeEffect(ctx context.Context, fallbackTenant string, resourceKind string, rawName string, quotaUnits int, ipBlocks int) (string, error) {
	if s.reservationPort == nil || rules.NormalizeResourceName(rawName) == "" {
		return "", nil
	}
	tenantID := observability.ExtractTenantID(ctx)
	if tenantID == "" {
		tenantID = fallbackTenant
	}
	if tenantID == "" {
		tenantID = "platform-default"
	}
	rec, err := s.reservationPort.Reserve(ctx, contracts.ReservationRequest{
		TenantID:       tenantID,
		ResourceKind:   resourceKind,
		RawName:        rawName,
		NormalizedName: rules.NormalizeResourceName(rawName),
		QuotaUnits:     quotaUnits,
		IPCIDRBlocks:   ipBlocks,
	})
	if err != nil {
		return "", err
	}
	return rec.ID, nil
}

func (s *SelfServiceProvisioningService) commitReservation(ctx context.Context, reservationID string) {
	if s.reservationPort == nil || reservationID == "" {
		return
	}
	_ = s.reservationPort.Commit(ctx, reservationID)
}

func (s *SelfServiceProvisioningService) releaseReservation(ctx context.Context, reservationID string) {
	if s.reservationPort == nil || reservationID == "" {
		return
	}
	_ = s.reservationPort.Release(ctx, reservationID)
}

type approvalPinContextKey struct{}

type approvalPinContext struct {
	approvalID      string
	plan            contracts.PinnedPlan
	maxPeriodBudget int
}

func WithApprovalPin(ctx context.Context, approvalID string, plan contracts.PinnedPlan, maxPeriodBudget int) context.Context {
	return context.WithValue(ctx, approvalPinContextKey{}, approvalPinContext{
		approvalID:      approvalID,
		plan:            plan,
		maxPeriodBudget: maxPeriodBudget,
	})
}

func (s *SelfServiceProvisioningService) verifyApprovalIfPresent(ctx context.Context) error {
	if s.approvalPort == nil {
		return nil
	}
	pin, ok := ctx.Value(approvalPinContextKey{}).(approvalPinContext)
	if !ok || pin.approvalID == "" {
		return nil
	}
	return s.approvalPort.VerifyAndConsumeApproval(ctx, pin.approvalID, pin.plan, pin.maxPeriodBudget)
}
