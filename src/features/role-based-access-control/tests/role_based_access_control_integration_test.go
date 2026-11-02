package tests

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
)

func TestRoleBasedAccessControlIntegration_LiveRedis(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:6379", 1*time.Second)
	if err != nil {
		t.Skip("Live Redis 7.4 unreachable")
		return
	}
	conn.Close()

	ctx := context.Background()
	adapter := rbacAdapters.NewRedisRevocationAdapter("127.0.0.1:6379", 2*time.Second)

	testSub := fmt.Sprintf("sub-canonical-%d", time.Now().UnixNano())
	testJTI := fmt.Sprintf("jti-canonical-%d", time.Now().UnixNano())

	revoked, err := adapter.IsRevoked(ctx, testSub, testJTI)
	if err != nil || revoked {
		t.Fatalf("expected not revoked initially, got revoked=%v, err=%v", revoked, err)
	}

	if err := adapter.RevokeSubject(ctx, testSub); err != nil {
		t.Fatalf("failed to revoke subject: %v", err)
	}

	revoked, err = adapter.IsRevoked(ctx, testSub, "")
	if err != nil || !revoked {
		t.Fatalf("expected subject revoked in Redis, got %v", revoked)
	}

	if err := adapter.RevokeJTI(ctx, testJTI); err != nil {
		t.Fatalf("failed to revoke JTI: %v", err)
	}

	revoked, err = adapter.IsRevoked(ctx, "", testJTI)
	if err != nil || !revoked {
		t.Fatalf("expected JTI revoked in Redis, got %v", revoked)
	}
}
