/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC REDIS REVOCATION & INVALIDATION ADAPTER

1. Architecture Role:
  - Step 11 Outbound Adapter: Implements ports.RevocationStorePort using github.com/redis/go-redis/v9.
  - Fails closed by returning explicit infrastructure errors on Redis unavailability.
  - Supports distributed PDP cache invalidation via Redis Pub/Sub channel ("rbac:cache:invalidate").
  - Enforces Tier-4 OpenTelemetry spans and zero inline comments inside function bodies.
*/
package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	rbacQueries "internal-developer-platform/src/features/role-based-access-control/internal/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
)

type RedisRevocationAdapter struct {
	client       *redis.Client
	addr         string
	mu           sync.RWMutex
	localRevoked map[string]time.Time
}

func NewRedisRevocationAdapter(addr string, timeout time.Duration) *RedisRevocationAdapter {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	var client *redis.Client
	if addr != "" {
		client = redis.NewClient(&redis.Options{
			Addr:         addr,
			DialTimeout:  timeout,
			ReadTimeout:  timeout,
			WriteTimeout: timeout,
		})
	}
	return &RedisRevocationAdapter{
		client:       client,
		addr:         addr,
		localRevoked: make(map[string]time.Time),
	}
}

func (a *RedisRevocationAdapter) isLocallyRevoked(key string) bool {
	if key == "" || a.localRevoked == nil {
		return false
	}
	a.mu.RLock()
	exp, ok := a.localRevoked[key]
	a.mu.RUnlock()
	return ok && time.Now().UTC().Before(exp)
}

func (a *RedisRevocationAdapter) markLocallyRevoked(key string, ttl time.Duration) {
	if key == "" || a.localRevoked == nil {
		return
	}
	a.mu.Lock()
	a.localRevoked[key] = time.Now().UTC().Add(ttl)
	a.mu.Unlock()
}

func (a *RedisRevocationAdapter) IsRevoked(ctx context.Context, subjectID string, jti string) (bool, error) {
	return observability.RunTraced(ctx, "redis.revocation.is_revoked", func(ctx context.Context) (bool, error) {
		if a.isLocallyRevoked("sub:"+subjectID) || a.isLocallyRevoked("jti:"+jti) {
			return true, nil
		}
		if a.client == nil {
			return false, fmt.Errorf("redis client is not configured")
		}
		if subjectID != "" {
			count, err := a.client.Exists(ctx, rbacQueries.BuildRevocationSubKey(subjectID)).Result()
			if err != nil {
				return false, fmt.Errorf("redis revocation subject lookup failed: %w", err)
			}
			if count > 0 {
				return true, nil
			}
		}
		if jti != "" {
			count, err := a.client.Exists(ctx, rbacQueries.BuildRevocationJTIKey(jti)).Result()
			if err != nil {
				return false, fmt.Errorf("redis revocation jti lookup failed: %w", err)
			}
			if count > 0 {
				return true, nil
			}
		}
		return false, nil
	})
}

func (a *RedisRevocationAdapter) RevokeSubject(ctx context.Context, subjectID string) error {
	return observability.WithSpan(ctx, "redis.revocation.revoke_subject", func(ctx context.Context) error {
		if a.client == nil {
			return fmt.Errorf("redis client is not configured")
		}
		ttl := time.Duration(rbacQueries.DefaultTTLSeconds) * time.Second
		if err := a.client.Set(ctx, rbacQueries.BuildRevocationSubKey(subjectID), rbacQueries.RevokedSentinelValue, ttl).Err(); err != nil {
			return fmt.Errorf("redis revoke subject failed: %w", err)
		}
		a.markLocallyRevoked("sub:"+subjectID, ttl)
		_ = a.client.Publish(ctx, constants.RBACInvalidationChannel, subjectID).Err()
		return nil
	})
}

func (a *RedisRevocationAdapter) RevokeJTI(ctx context.Context, jti string) error {
	return observability.WithSpan(ctx, "redis.revocation.revoke_jti", func(ctx context.Context) error {
		if a.client == nil {
			return fmt.Errorf("redis client is not configured")
		}
		ttl := time.Duration(rbacQueries.DefaultTTLSeconds) * time.Second
		if err := a.client.Set(ctx, rbacQueries.BuildRevocationJTIKey(jti), rbacQueries.RevokedSentinelValue, ttl).Err(); err != nil {
			return fmt.Errorf("redis revoke jti failed: %w", err)
		}
		return nil
	})
}

func (a *RedisRevocationAdapter) PublishInvalidation(ctx context.Context, subjectID string) error {
	return observability.WithSpan(ctx, "redis.revocation.publish_invalidation", func(ctx context.Context) error {
		if a.client == nil {
			return fmt.Errorf("redis client is not configured")
		}
		if err := a.client.Publish(ctx, constants.RBACInvalidationChannel, subjectID).Err(); err != nil {
			return fmt.Errorf("redis publish invalidation failed: %w", err)
		}
		return nil
	})
}

func (a *RedisRevocationAdapter) PublishInvalidationWithAck(ctx context.Context, subjectID string, expectedNodes int, ackTimeout time.Duration) (int, error) {
	return observability.RunTracedClient(ctx, "redis.revocation.publish_invalidation_ack", func(ctx context.Context) (int, error) {
		if a.client == nil {
			return 0, fmt.Errorf("redis client is not configured")
		}
		if ackTimeout <= 0 {
			ackTimeout = 500 * time.Millisecond
		}
		if expectedNodes <= 0 {
			expectedNodes = 1
		}
		ackSub := a.client.Subscribe(ctx, constants.RBACInvalidationAckChannel)
		defer ackSub.Close()
		if err := a.client.Publish(ctx, constants.RBACInvalidationChannel, subjectID).Err(); err != nil {
			return 0, fmt.Errorf("redis publish invalidation failed: %w", err)
		}
		timer := time.NewTimer(ackTimeout)
		defer timer.Stop()
		acks := 0
		ch := ackSub.Channel()
		for acks < expectedNodes {
			select {
			case <-ctx.Done():
				return acks, ctx.Err()
			case <-timer.C:
				return acks, nil
			case msg, ok := <-ch:
				if !ok {
					return acks, nil
				}
				if msg.Payload == subjectID {
					acks++
				}
			}
		}
		return acks, nil
	})
}

func (a *RedisRevocationAdapter) TryAcquireLeaderLease(ctx context.Context, leaseKey string, ttl time.Duration) (bool, error) {
	if a.client == nil {
		return true, nil
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	acquired, err := a.client.SetNX(ctx, "rbac:lease:"+leaseKey, "1", ttl).Result()
	if err != nil {
		return false, err
	}
	return acquired, nil
}

func (a *RedisRevocationAdapter) SubscribeInvalidation(ctx context.Context, onInvalidate func(subjectID string)) {
	if a.client == nil {
		return
	}
	pubsub := a.client.Subscribe(ctx, constants.RBACInvalidationChannel)
	go func() {
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg.Payload != "" && onInvalidate != nil {
					onInvalidate(msg.Payload)
					_ = a.client.Publish(ctx, constants.RBACInvalidationAckChannel, msg.Payload).Err()
				}
			}
		}
	}()
}
