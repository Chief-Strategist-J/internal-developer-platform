package kubernetesworkloadprovisioning

import (
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/adapters"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/service"
)

type Service = service.WorkloadProvisioningService
type Command = contracts.WorkloadProvisionCommand
type Result = contracts.WorkloadProvisionResult
type Record = contracts.WorkloadProvisionRecord
type StorePort = ports.WorkloadStorePort
type ClientPort = ports.K8sClientPort

var NewService = service.NewWorkloadProvisioningService
var NewMemoryStore = adapters.NewMemoryWorkloadRepositoryAdapter
var NewPostgresStore = adapters.NewPostgresWorkloadRepositoryAdapter
var NewK8sClient = adapters.NewKubernetesClientAdapter
var NewLiveK8sClient = adapters.NewLiveKubernetesClientAdapter


