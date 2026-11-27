package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"internal-developer-platform/src/features/policy-as-code/internal/contracts"
	"internal-developer-platform/src/features/policy-as-code/internal/infrastructure/adapters"
)

const (
	TestTenantA        = "11111111-1111-1111-1111-111111111111"
	TestTenantB        = "22222222-2222-2222-2222-222222222222"
	TestPolicyID1      = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	TestPolicyID2      = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	TestPublicID1      = "POL-001"
	TestPublicID2      = "POL-002"
	TestCorrelationID  = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	TestExternalID1    = "EXT-001"
	TestExternalID2    = "EXT-002"
	TestPolicyName     = "Restricted Pod Security"
	TestRuleExpression = "request.spec.containers.all(c, !c.securityContext.privileged)"
	TestDetectionPair  = "SEC-001:SEC-002"
	TestExceptionID    = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	TestExcPublicID    = "EXC-001"
	TestWorkloadID     = "payment-processor"
	TestNamespace      = "prod-apps"
	TestApproverID     = "security-lead@company.internal"
	TestJustification  = "Legacy daemon requires root privilege until Q2 migration"
)

func createTestPolicy(tenantID, id, publicID, extID string, state contracts.PolicyLifecycleState) contracts.PolicyRecord {
	now := time.Now().UTC()
	return contracts.PolicyRecord{
		ID:               id,
		PublicID:         publicID,
		TenantID:         tenantID,
		Source:           contracts.PolicySourceGit,
		ExternalID:       extID,
		CorrelationID:    TestCorrelationID,
		Name:             TestPolicyName,
		Description:      "Bans privileged containers across workloads",
		EnforcementPoint: contracts.EnforcementPointCEL,
		Severity:         contracts.SeverityCritical,
		LifecycleState:   state,
		RuleExpression:   TestRuleExpression,
		Exceptable:       true,
		DetectionPair:    TestDetectionPair,
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func createTestException(tenantID, excID, pubID, policyID string) contracts.PolicyExceptionRecord {
	now := time.Now().UTC()
	return contracts.PolicyExceptionRecord{
		ID:            excID,
		PublicID:      pubID,
		TenantID:      tenantID,
		PolicyID:      policyID,
		WorkloadID:    TestWorkloadID,
		Namespace:     TestNamespace,
		ApproverID:    TestApproverID,
		Justification: TestJustification,
		Status:        contracts.ExceptionStatusActive,
		ExpiresAt:     now.Add(24 * time.Hour),
		CorrelationID: TestCorrelationID,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func TestPolicyValidation(t *testing.T) {
	p := createTestPolicy(TestTenantA, TestPolicyID1, TestPublicID1, TestExternalID1, contracts.LifecycleDraft)
	require.NoError(t, p.Validate())

	pEmptyName := p
	pEmptyName.Name = ""
	assert.Error(t, pEmptyName.Validate())

	pInvalidSource := p
	pInvalidSource.Source = "UNKNOWN"
	assert.Error(t, pInvalidSource.Validate())

	pInvalidEnforcement := p
	pInvalidEnforcement.EnforcementPoint = "INVALID"
	assert.Error(t, pInvalidEnforcement.Validate())

	pInvalidSeverity := p
	pInvalidSeverity.Severity = "LOW"
	assert.Error(t, pInvalidSeverity.Validate())

	pInvalidLifecycle := p
	pInvalidLifecycle.LifecycleState = "ACTIVE"
	assert.Error(t, pInvalidLifecycle.Validate())

	exc := createTestException(TestTenantA, TestExceptionID, TestExcPublicID, TestPolicyID1)
	require.NoError(t, exc.Validate())

	excShortJustification := exc
	excShortJustification.Justification = "short"
	assert.Error(t, excShortJustification.Validate())

	excExpiredBeforeCreated := exc
	excExpiredBeforeCreated.ExpiresAt = exc.CreatedAt.Add(-1 * time.Hour)
	assert.Error(t, excExpiredBeforeCreated.Validate())
}

func TestMemoryRepository_PolicyCRUDAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()

	policy1 := createTestPolicy(TestTenantA, TestPolicyID1, TestPublicID1, TestExternalID1, contracts.LifecycleDraft)
	err := repo.SavePolicy(ctx, policy1)
	require.NoError(t, err)

	err = repo.SavePolicy(ctx, policy1)
	assert.Error(t, err)

	found, err := repo.GetPolicyByID(ctx, TestTenantA, TestPolicyID1)
	require.NoError(t, err)
	assert.Equal(t, TestPublicID1, found.PublicID)

	_, err = repo.GetPolicyByID(ctx, TestTenantB, TestPolicyID1)
	assert.Error(t, err)

	foundPub, err := repo.GetPolicyByPublicID(ctx, TestTenantA, TestPublicID1)
	require.NoError(t, err)
	assert.Equal(t, TestPolicyID1, foundPub.ID)

	_, err = repo.GetPolicyByPublicID(ctx, TestTenantB, TestPublicID1)
	assert.Error(t, err)

	policy2 := createTestPolicy(TestTenantA, TestPolicyID2, TestPublicID2, TestExternalID2, contracts.LifecycleAudit)
	require.NoError(t, repo.SavePolicy(ctx, policy2))

	list, err := repo.ListPoliciesKeyset(ctx, contracts.KeysetFilterCriteria{
		TenantID: TestTenantA,
		CursorID: "",
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Len(t, list, 2)

	activeCEL, err := repo.ListActiveCELPolicies(ctx, TestTenantA)
	require.NoError(t, err)
	assert.Len(t, activeCEL, 1)
	assert.Equal(t, TestPolicyID2, activeCEL[0].ID)

	updateErr := repo.UpdatePolicy(ctx, policy1, 999)
	assert.Error(t, updateErr)

	policy1.Description = "Updated description"
	require.NoError(t, repo.UpdatePolicy(ctx, policy1, 1))

	updated, err := repo.GetPolicyByID(ctx, TestTenantA, TestPolicyID1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.Version)
	assert.Equal(t, "Updated description", updated.Description)

	illegalStateErr := repo.UpdatePolicyLifecycle(ctx, TestTenantA, TestPolicyID1, contracts.LifecycleDeny, 2)
	assert.Error(t, illegalStateErr)

	require.NoError(t, repo.UpdatePolicyLifecycle(ctx, TestTenantA, TestPolicyID1, contracts.LifecycleCIOnly, 2))
	inCI, err := repo.GetPolicyByID(ctx, TestTenantA, TestPolicyID1)
	require.NoError(t, err)
	assert.Equal(t, contracts.LifecycleCIOnly, inCI.LifecycleState)
	assert.Equal(t, int64(3), inCI.Version)

	require.NoError(t, repo.SoftDeletePolicy(ctx, TestTenantA, TestPolicyID1, 3))
	_, err = repo.GetPolicyByID(ctx, TestTenantA, TestPolicyID1)
	assert.Error(t, err)
}

func TestMemoryRepository_ExceptionsLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()

	exc := createTestException(TestTenantA, TestExceptionID, TestExcPublicID, TestPolicyID1)
	err := repo.SaveException(ctx, exc)
	assert.Error(t, err)

	policy1 := createTestPolicy(TestTenantA, TestPolicyID1, TestPublicID1, TestExternalID1, contracts.LifecycleDeny)
	require.NoError(t, repo.SavePolicy(ctx, policy1))

	require.NoError(t, repo.SaveException(ctx, exc))

	err = repo.SaveException(ctx, exc)
	assert.Error(t, err)

	foundExc, err := repo.GetExceptionByID(ctx, TestTenantA, TestExceptionID)
	require.NoError(t, err)
	assert.Equal(t, contracts.ExceptionStatusActive, foundExc.Status)

	activeList, err := repo.GetActiveExceptionsForWorkload(ctx, TestTenantA, TestNamespace, TestWorkloadID)
	require.NoError(t, err)
	assert.Len(t, activeList, 1)

	emptyList, err := repo.GetActiveExceptionsForWorkload(ctx, TestTenantB, TestNamespace, TestWorkloadID)
	require.NoError(t, err)
	assert.Empty(t, emptyList)

	err = repo.UpdateExceptionStatus(ctx, TestTenantA, TestExceptionID, contracts.ExceptionStatusRevoked, 999)
	assert.Error(t, err)

	require.NoError(t, repo.UpdateExceptionStatus(ctx, TestTenantA, TestExceptionID, contracts.ExceptionStatusRevoked, 1))

	revokedList, err := repo.GetActiveExceptionsForWorkload(ctx, TestTenantA, TestNamespace, TestWorkloadID)
	require.NoError(t, err)
	assert.Empty(t, revokedList)
}

func TestMemoryRepository_ReconcileExpiredExceptions(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()

	now := time.Now().UTC()
	policy1 := createTestPolicy(TestTenantA, TestPolicyID1, TestPublicID1, TestExternalID1, contracts.LifecycleDraft)
	require.NoError(t, repo.SavePolicy(ctx, policy1))

	exc := createTestException(TestTenantA, TestExceptionID, TestExcPublicID, TestPolicyID1)
	exc.CreatedAt = now.Add(-2 * time.Hour)
	exc.UpdatedAt = now.Add(-2 * time.Hour)
	exc.ExpiresAt = now.Add(-1 * time.Hour)
	require.NoError(t, repo.SaveException(ctx, exc))

	reconciled, err := repo.ReconcileExpiredExceptions(ctx, TestTenantA, now)
	require.NoError(t, err)
	assert.Equal(t, 1, reconciled)

	foundExc, err := repo.GetExceptionByID(ctx, TestTenantA, TestExceptionID)
	require.NoError(t, err)
	assert.Equal(t, contracts.ExceptionStatusExpired, foundExc.Status)

	reconciledSecond, err := repo.ReconcileExpiredExceptions(ctx, TestTenantA, now)
	require.NoError(t, err)
	assert.Equal(t, 0, reconciledSecond)
}

func TestMemoryRepository_ViolationsLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()

	now := time.Now().UTC()
	v := contracts.PolicyViolationRecord{
		ID:               "viol-001",
		PublicID:         "viol_pub_001",
		TenantID:         TestTenantA,
		PolicyID:         TestPolicyID1,
		Source:           "ADMISSION",
		ExternalID:       "EXT-VIOL-001",
		CorrelationID:    TestCorrelationID,
		WorkloadID:       TestWorkloadID,
		Namespace:        TestNamespace,
		Severity:         contracts.SeverityCritical,
		EnforcementPoint: contracts.EnforcementPointCEL,
		Status:           contracts.ViolationStatusActive,
		Message:          "Privileged container detected",
		DetectionPair:    "PC audit",
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	require.NoError(t, repo.SaveViolation(ctx, v))

	list, err := repo.ListActiveViolationsByPolicy(ctx, TestTenantA, TestPolicyID1)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "viol-001", list[0].ID)

	count, err := repo.CountActiveViolationsByPolicy(ctx, TestTenantA, TestPolicyID1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	err = repo.ResolveViolation(ctx, TestTenantA, "viol-001", 999)
	assert.Error(t, err)

	require.NoError(t, repo.ResolveViolation(ctx, TestTenantA, "viol-001", 1))

	countAfter, err := repo.CountActiveViolationsByPolicy(ctx, TestTenantA, TestPolicyID1)
	require.NoError(t, err)
	assert.Equal(t, 0, countAfter)
}
