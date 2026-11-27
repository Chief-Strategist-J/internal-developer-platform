/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME SERVICE

1. Service Orchestrator:
  - Registers and supersedes regime policies per tenant.
  - Falls back to declarative config baseline if no dynamic record exists for the tenant.
  - Enforces W3C distributed tracing spans on all queries.
  - Zero comments inside method bodies.
*/
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/platform-regime/internal/contracts"
	"internal-developer-platform/src/features/platform-regime/internal/domain/rules"
	"internal-developer-platform/src/features/platform-regime/internal/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PlatformRegimeService struct {
	repo            ports.PlatformRegimeRepositoryPort
	defaultBaseline contracts.PlatformRegimePolicyRecord
}

func NewPlatformRegimeService(repo ports.PlatformRegimeRepositoryPort, baseline contracts.PlatformRegimePolicyRecord) *PlatformRegimeService {
	return &PlatformRegimeService{
		repo:            repo,
		defaultBaseline: baseline,
	}
}

func (s *PlatformRegimeService) RegisterRegimePolicy(ctx context.Context, cmd contracts.CreateRegimePolicyCommand) (*contracts.PlatformRegimePolicyRecord, error) {
	return observability.RunTraced(ctx, "platform_regime.register", func(ctx context.Context) (*contracts.PlatformRegimePolicyRecord, error) {
		if err := contracts.ValidateCreateRegimePolicyCommand(cmd); err != nil {
			return nil, err
		}

		rawID := uuid.New().String()
		publicID := contracts.ComputeRegimePolicyPublicID(rawID)

		correlationID := cmd.CorrelationID
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		source := cmd.Source
		if source == "" {
			source = "api"
		}

		now := time.Now().UTC()
		record := contracts.PlatformRegimePolicyRecord{
			ID:                   rawID,
			PublicID:             publicID,
			TenantID:             cmd.TenantID,
			Source:               source,
			ExternalID:           cmd.ExternalID,
			CorrelationID:        correlationID,
			Jurisdiction:         cmd.Jurisdiction,
			AuditRetentionDays:   cmd.AuditRetentionDays,
			DataResidencyRegions: cmd.DataResidencyRegions,
			KMSKeyCustodyMode:    cmd.KMSKeyCustodyMode,
			RTOMinutesTier0:      cmd.RTOMinutesTier0,
			RPOMinutesTier0:      cmd.RPOMinutesTier0,
			RTOMinutesTier1:      cmd.RTOMinutesTier1,
			RPOMinutesTier1:      cmd.RPOMinutesTier1,
			ClusterMode:          cmd.ClusterMode,
			PrimaryRegionCount:   cmd.PrimaryRegionCount,
			StandbyRegionCount:   cmd.StandbyRegionCount,
			TenantIsolationMode:  cmd.TenantIsolationMode,
			Status:               contracts.RegimeStatusActive,
			Version:              1,
			CreatedAt:            now,
			UpdatedAt:            now,
		}

		if err := s.repo.Save(ctx, record); err != nil {
			return nil, err
		}

		return &record, nil
	}, attribute.String("tenant.id", cmd.TenantID), attribute.String("jurisdiction", cmd.Jurisdiction))
}

func (s *PlatformRegimeService) GetActiveRegimePolicy(ctx context.Context, tenantID string) (*contracts.PlatformRegimePolicyRecord, error) {
	return observability.RunTraced(ctx, "platform_regime.get_active", func(ctx context.Context) (*contracts.PlatformRegimePolicyRecord, error) {
		if tenantID == "" {
			return nil, errors.NewValidationError("tenantId must not be empty")
		}

		record, err := s.repo.GetActive(ctx, tenantID)
		if err == nil && record != nil {
			return record, nil
		}

		fallback := s.defaultBaseline
		fallback.TenantID = tenantID
		return &fallback, nil
	}, attribute.String("tenant.id", tenantID))
}

func (s *PlatformRegimeService) EvaluateWorkloadCompliance(ctx context.Context, tenantID string, input contracts.WorkloadComplianceInput) (bool, []contracts.ComplianceViolation, error) {
	regime, err := s.GetActiveRegimePolicy(ctx, tenantID)
	if err != nil || regime == nil {
		return true, nil, nil
	}
	ok, violations := rules.ValidateWorkloadAgainstRegime(input, *regime)
	return ok, violations, nil
}
