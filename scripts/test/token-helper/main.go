package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: token-helper <secret> <subID> <tenantID> <role1,role2...>\n")
		os.Exit(1)
	}

	secret := os.Args[1]
	subID := os.Args[2]
	tenantID := os.Args[3]
	rolesArg := os.Args[4]

	var roles []rbacContracts.RoleName
	var roleStrs []string
	if err := json.Unmarshal([]byte(rolesArg), &roleStrs); err == nil {
		for _, r := range roleStrs {
			roles = append(roles, rbacContracts.RoleName(r))
		}
	} else {
		roles = append(roles, rbacContracts.RoleName(rolesArg))
	}

	verifier := rbacAdapters.NewJWTTokenVerifierAdapter(secret, "internal-developer-platform")
	token, err := verifier.IssueToken(rbacContracts.Subject{
		ID:        subID,
		TenantID:  tenantID,
		Roles:     roles,
		ExpiresAt: time.Now().Add(2 * time.Hour),
		JTI:       fmt.Sprintf("cli-jti-%d", time.Now().UnixNano()),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to issue token: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(token)
}
