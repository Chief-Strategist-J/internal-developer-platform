// Package disasterrecovery provides the public facade for Disaster Recovery.
//
// Architectural Blueprint:
// - Step 13 Public Facade: Shields internal infrastructure adapters and queries.
// - Exports strictly the domain orchestrator Service, public DTOs, and Port abstractions.
// - External callers and dependency injectors must import concrete adapters from
//   infrastructure/adapters directly.
package disasterrecovery

import (
	"internal-developer-platform/src/features/disaster-recovery/contracts"
	"internal-developer-platform/src/features/disaster-recovery/infrastructure/ports"
	"internal-developer-platform/src/features/disaster-recovery/service"
)

type Service = service.DisasterRecoveryService
type DRPlanRecord = contracts.DRPlanRecord
type CreateDRPlanCommand = contracts.CreateDRPlanCommand
type ReadinessCheckResult = contracts.ReadinessCheckResult
type DRIncidentRecord = contracts.DRIncidentRecord
type DRFailoverOperationRecord = contracts.DRFailoverOperationRecord
type DRFailbackOperationRecord = contracts.DRFailbackOperationRecord
type DrillRunRecord = contracts.DrillRunRecord
type FencingEpochRecord = contracts.FencingEpochRecord

type DRPlanStorePort = ports.DRPlanStorePort
type DatabaseFailoverPort = ports.DatabaseFailoverPort
type TrafficRoutingPort = ports.TrafficRoutingPort
type ClusterFailoverPort = ports.ClusterFailoverPort
type SecretReplicationPort = ports.SecretReplicationPort

var NewService = service.NewDisasterRecoveryService

