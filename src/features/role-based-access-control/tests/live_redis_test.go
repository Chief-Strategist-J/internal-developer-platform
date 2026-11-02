package tests

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
)

func TestLiveRedisRevocationAdapter(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:6379", 1*time.Second)
	if err != nil {
		t.Skip("Live Redis 7.4 (127.0.0.1:6379) not reachable, skipping live infrastructure test")
		return
	}
	conn.Close()

	ctx := context.Background()
	adapter := rbacAdapters.NewRedisRevocationAdapter("127.0.0.1:6379", 2*time.Second)

	testSub := fmt.Sprintf("test-sub-%d", time.Now().UnixNano())
	testJTI := fmt.Sprintf("test-jti-%d", time.Now().UnixNano())

	// Initially not revoked
	revoked, err := adapter.IsRevoked(ctx, testSub, testJTI)
	if err != nil {
		t.Fatalf("failed to query live Redis: %v", err)
	}
	if revoked {
		t.Errorf("expected subject to not be revoked initially")
	}

	// Revoke subject in real Redis
	if err := adapter.RevokeSubject(ctx, testSub); err != nil {
		t.Fatalf("failed to revoke subject in live Redis: %v", err)
	}

	// Now must be revoked
	revoked, err = adapter.IsRevoked(ctx, testSub, "")
	if err != nil {
		t.Fatalf("failed to query live Redis after revocation: %v", err)
	}
	if !revoked {
		t.Errorf("expected subject to be reported as revoked in live Redis")
	}

	// Revoke JTI in real Redis
	if err := adapter.RevokeJTI(ctx, testJTI); err != nil {
		t.Fatalf("failed to revoke JTI in live Redis: %v", err)
	}

	revoked, err = adapter.IsRevoked(ctx, "", testJTI)
	if err != nil {
		t.Fatalf("failed to query live Redis for revoked JTI: %v", err)
	}
	if !revoked {
		t.Errorf("expected JTI to be reported as revoked in live Redis")
	}
}
