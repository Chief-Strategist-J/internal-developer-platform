package tests

import (
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
)

func TestValidateSyncCommand(t *testing.T) {
	valid := contracts.SyncCommand{
		ID:             "reconcile-001",
		TenantID:       "tenant-alpha",
		ServiceID:      "service-omega",
		AppName:        "my-argo-app",
		TargetRevision: "main",
		Prune:          true,
		DryRun:         false,
		IdempotencyKey: "key-12345",
	}

	if err := contracts.ValidateSyncCommand(valid); err != nil {
		t.Fatalf("expected valid command to pass, got: %v", err)
	}

	invalidAppName := valid
	invalidAppName.AppName = "My_App_Invalid"
	if err := contracts.ValidateSyncCommand(invalidAppName); err == nil {
		t.Errorf("expected error on non-RFC-1123 appName")
	}

	emptyTenant := valid
	emptyTenant.TenantID = ""
	if err := contracts.ValidateSyncCommand(emptyTenant); err == nil {
		t.Errorf("expected error on empty tenantId")
	}

	emptyIdempotency := valid
	emptyIdempotency.IdempotencyKey = ""
	if err := contracts.ValidateSyncCommand(emptyIdempotency); err == nil {
		t.Errorf("expected error on empty idempotency key")
	}
}
