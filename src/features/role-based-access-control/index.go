package rbac

import (
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
)

type Service = rbacService.RBACService
type Subject = rbacContracts.Subject
type RoleName = rbacContracts.RoleName
type Permission = rbacContracts.Permission
type Resource = rbacContracts.Resource
type Action = rbacContracts.Action
