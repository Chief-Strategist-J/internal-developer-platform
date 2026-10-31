package servicecatalog

import (
	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
	catalogService "internal-developer-platform/src/features/service-catalog/service"
)

type Service = catalogService.ServiceCatalogService
type Descriptor = catalogContracts.ServiceDescriptor
type Status = catalogContracts.ServiceStatus
