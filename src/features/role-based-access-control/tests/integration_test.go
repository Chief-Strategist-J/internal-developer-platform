package tests

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
)

// --- INTEGRATION TEST 1: Live Redis Revocation Concurrency & High Throughput ---
func TestIntegration_LiveRedis_ConcurrentRevocations(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:6379", 1*time.Second)
	if err != nil {
		t.Skip("Live Redis 7.4 (127.0.0.1:6379) unreachable, skipping live integration suite")
		return
	}
	conn.Close()

	ctx := context.Background()
	redisAdapter := rbacAdapters.NewRedisRevocationAdapter("127.0.0.1:6379", 3*time.Second)
	tokenVerifier := rbacAdapters.NewJWTTokenVerifierAdapter("integration-secret-test-key", "integration-issuer")
	svc := rbacService.NewRBACService(tokenVerifier, redisAdapter)

	workerCount := 30
	var wg sync.WaitGroup
	errChan := make(chan error, workerCount*2)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			subID := fmt.Sprintf("concurrent-sub-%d-%d", workerID, time.Now().UnixNano())
			jtiID := fmt.Sprintf("concurrent-jti-%d-%d", workerID, time.Now().UnixNano())

			// 1. Verify not revoked initially
			revoked, err := redisAdapter.IsRevoked(ctx, subID, jtiID)
			if err != nil || revoked {
				errChan <- fmt.Errorf("worker %d: expected false initially, got revoked=%v, err=%v", workerID, revoked, err)
				return
			}

			// 2. Revoke Subject
			if err := svc.RevokeSubject(ctx, subID); err != nil {
				errChan <- fmt.Errorf("worker %d: failed to revoke subject: %v", workerID, err)
				return
			}

			// 3. Confirm Subject Revoked
			revoked, err = redisAdapter.IsRevoked(ctx, subID, "")
			if err != nil || !revoked {
				errChan <- fmt.Errorf("worker %d: expected subject to be revoked, got revoked=%v, err=%v", workerID, revoked, err)
				return
			}

			// 4. Revoke JTI
			if err := redisAdapter.RevokeJTI(ctx, jtiID); err != nil {
				errChan <- fmt.Errorf("worker %d: failed to revoke jti: %v", workerID, err)
				return
			}

			// 5. Confirm JTI Revoked
			revoked, err = redisAdapter.IsRevoked(ctx, "", jtiID)
			if err != nil || !revoked {
				errChan <- fmt.Errorf("worker %d: expected jti to be revoked, got revoked=%v, err=%v", workerID, revoked, err)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Fatalf("Concurrent Redis integration test failure: %v", err)
	}
}
