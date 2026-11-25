package gitopsreconciliation

import (
	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/adapters"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/ports"
	"internal-developer-platform/src/features/gitops-reconciliation/service"
)

type Service = service.GitOpsReconciliationService
type Command = contracts.SyncCommand
type Result = contracts.SyncResult
type Record = contracts.ReconciliationRecord
type OutboxEvent = contracts.OutboxEvent
type StorePort = ports.GitOpsStorePort
type ClientPort = ports.ArgoCDClientPort

var NewService = service.NewGitOpsReconciliationService
var NewSecretAdapterWithDB = adapters.NewConfigurableSecretAdapterWithDB
var NewOutboxAdapterWithDB = adapters.NewPostgresOutboxAdapter
var NewMemoryOutboxAdapter = adapters.NewMemoryOutboxAdapter
var NewMemoryStore = adapters.NewMemoryGitOpsRepositoryAdapter
var NewPostgresStore = adapters.NewPostgresGitOpsRepositoryAdapter
var NewArgoCDClient = adapters.NewArgoCDClientAdapter
var NewLiveArgoCDClient = adapters.NewLiveArgoCDClientAdapter
