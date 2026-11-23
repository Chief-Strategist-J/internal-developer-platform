/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY AS CODE INDEX

1. Package Entrypoint:
   - Exposes clean constructors and types for the policy-as-code feature.
   - Zero comments inside method bodies.
*/
package policyascode

import (
	"database/sql"

	"internal-developer-platform/src/features/policy-as-code/infrastructure/adapters"
	"internal-developer-platform/src/features/policy-as-code/infrastructure/ports"
	"internal-developer-platform/src/features/policy-as-code/service"
)

type PolicyAsCodeService = service.PolicyAsCodeService
type PolicyRepositoryPort = ports.PolicyRepositoryPort

func NewService(repo ports.PolicyRepositoryPort) *PolicyAsCodeService {
	return service.NewPolicyAsCodeService(repo)
}

func NewMemoryRepository() ports.PolicyRepositoryPort {
	return adapters.NewMemoryPolicyRepositoryAdapter()
}

func NewPostgresRepository(db *sql.DB) ports.PolicyRepositoryPort {
	return adapters.NewPostgresPolicyRepositoryAdapter(db)
}

