package tests

import (
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
)

func TestEvaluateSourceTrust(t *testing.T) {
	if err := rules.EvaluateSourceTrust(false, true, true, true); err == nil {
		t.Fatal("expected error when prod project does not enforce signature verification")
	}
	if err := rules.EvaluateSourceTrust(true, false, true, true); err == nil {
		t.Fatal("expected error when commit is unsigned")
	}
	if err := rules.EvaluateSourceTrust(true, true, false, true); err == nil {
		t.Fatal("expected error when commit key is not allowed")
	}
	if err := rules.EvaluateSourceTrust(true, true, true, false); err == nil {
		t.Fatal("expected error when commit lacks role-based review")
	}
	if err := rules.EvaluateSourceTrust(true, true, true, true); err != nil {
		t.Fatalf("unexpected error for trusted commit: %v", err)
	}
}

func TestEvaluateSyncWindow(t *testing.T) {
	if err := rules.EvaluateSyncWindow(false, false, true, false, false); err == nil {
		t.Fatal("expected error during deny window for automated sync")
	}
	if err := rules.EvaluateSyncWindow(false, false, true, true, true); err != nil {
		t.Fatalf("expected manual sync to be allowed when window permits it: %v", err)
	}
	if err := rules.EvaluateSyncWindow(true, false, false, false, false); err == nil {
		t.Fatal("expected error outside all allow windows")
	}
	if err := rules.EvaluateSyncWindow(true, true, false, false, false); err != nil {
		t.Fatalf("unexpected error inside allow window: %v", err)
	}
}

func TestShouldRetryFailedSHA(t *testing.T) {
	if rules.ShouldRetryFailedSHA("abc123", "abc123") {
		t.Fatal("expected false when currentSHA matches lastFailedSHA")
	}
	if !rules.ShouldRetryFailedSHA("def456", "abc123") {
		t.Fatal("expected true for a new fix-forward SHA")
	}
}

func TestValidateSharedResourceOwnership(t *testing.T) {
	if err := rules.ValidateSharedResourceOwnership("app-a", "app-b"); err == nil {
		t.Fatal("expected conflict error when resource is owned by another application")
	}
	if err := rules.ValidateSharedResourceOwnership("app-a", "app-a"); err != nil {
		t.Fatalf("unexpected error for owning application: %v", err)
	}
	if err := rules.ValidateSharedResourceOwnership("", "app-a"); err != nil {
		t.Fatalf("unexpected error for unowned resource: %v", err)
	}
}
