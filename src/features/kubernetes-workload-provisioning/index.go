// Package kubernetesworkloadprovisioning provides the public facade for the
// Kubernetes Workload Provisioning feature.
//
// Architectural Blueprint:
// - Step 13 Public Facade: Shields internal infrastructure adapters and queries.
// - Exports strictly the domain orchestrator Service, public DTOs, and Port abstractions.
// - External callers and dependency injectors must import concrete adapters from
//   infrastructure/adapters directly.
package kubernetesworkloadprovisioning

import (
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/service"
)

type Service = service.WorkloadProvisioningService
type Command = contracts.WorkloadProvisionCommand
type Result = contracts.WorkloadProvisionResult
type Record = contracts.WorkloadProvisionRecord
type StorePort = ports.WorkloadStorePort
type ClientPort = ports.K8sClientPort
type GitOpsPort = ports.GitOpsDeliveryPort
type KmsPort = ports.KmsEncryptionPort

var NewService = service.NewWorkloadProvisioningService
