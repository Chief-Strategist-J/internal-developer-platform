package queries

import (
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
)

type RBACQueries struct{}

func NewRBACQueries() *RBACQueries {
	return &RBACQueries{}
}

func (q *RBACQueries) GetRolePermissions(roleName rbacContracts.RoleName) []rbacContracts.Permission {
	if perms, exists := rules.DefaultRoleDefinitions[roleName]; exists {
		return perms
	}
	return []rbacContracts.Permission{}
}

func (q *RBACQueries) ListAvailableRoles() []rbacContracts.RoleName {
	var roles []rbacContracts.RoleName
	for r := range rules.DefaultRoleDefinitions {
		roles = append(roles, r)
	}
	return roles
}
