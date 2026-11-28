/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING SWEEPER LIFECYCLE (V.3 role 16, Section C)

 1. Role:
    - Periodically executes Section C sweeper runs: untracked intent detection, duplicate scan, and DNS cooldown validation.
    - Prevents multi-replica race conditions via distributed Redis leader lease ("ssp:lease:sweeper").
 2. Invariants:
    - Constructors and services MUST NOT spawn goroutines or tickers (G-11).
    - Timer loop executes exclusively inside Start() and terminates on Stop() or context cancel.
    - Zero mid-function inline comments below this header.
*/
package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/service"
	"internal-developer-platform/src/infra/observability"
)

const (
	defaultSweeperInterval = 24 * time.Hour
	sweeperLeaseKey        = "ssp:lease:sweeper"
	sweeperLeaseTTL        = 15 * time.Minute
)

type SelfServiceSweeperLifecycle struct {
	svc         *service.SelfServiceProvisioningService
	redisClient *redis.Client
	interval    time.Duration
	ready       atomic.Bool
	cancel      context.CancelFunc
	done        chan struct{}
}

func NewSelfServiceSweeperLifecycle(
	svc *service.SelfServiceProvisioningService,
	redisClient *redis.Client,
	interval time.Duration,
) *SelfServiceSweeperLifecycle {
	if interval <= 0 {
		interval = defaultSweeperInterval
	}
	return &SelfServiceSweeperLifecycle{
		svc:         svc,
		redisClient: redisClient,
		interval:    interval,
		done:        make(chan struct{}),
	}
}

func (l *SelfServiceSweeperLifecycle) Name() string {
	return "selfservice.sweeper"
}

func (l *SelfServiceSweeperLifecycle) Start(ctx context.Context) error {
	child, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	go l.run(child)
	return nil
}

func (l *SelfServiceSweeperLifecycle) Stop(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	select {
	case <-l.done:
	case <-ctx.Done():
	}
	return nil
}

func (l *SelfServiceSweeperLifecycle) Ready() bool {
	return l.ready.Load()
}

func (l *SelfServiceSweeperLifecycle) run(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	l.runCycleWithLease(ctx)
	l.ready.Store(true)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.runCycleWithLease(ctx)
		}
	}
}

func (l *SelfServiceSweeperLifecycle) runCycleWithLease(ctx context.Context) {
	logger := observability.GetLogger()
	if l.redisClient != nil {
		acquired, err := l.redisClient.SetNX(ctx, sweeperLeaseKey, "leader", sweeperLeaseTTL).Result()
		if err != nil {
			logger.Warn(ctx, "ssp sweeper failed to query leader lease; running cycle unleased", observability.String("error", err.Error()))
		} else if !acquired {
			logger.Info(ctx, "ssp sweeper leader lease held by peer replica; skipping cycle")
			return
		}
	}

	_ = observability.WithSpan(ctx, "self_service.sweeper.cycle", func(ctx context.Context) error {
		report, err := l.svc.RunSweeper(ctx)
		if err != nil {
			logger.Error(ctx, "ssp sweeper cycle execution failed", observability.String("error", err.Error()))
			return err
		}
		metrics := observability.GetSSPMetrics()
		metrics.SweeperOrphanedDirty.Set(float64(len(report.UntrackedIntents)))
		metrics.SweeperDuplicates.Set(float64(len(report.DuplicateTargets)))
		logger.Info(ctx, "ssp sweeper cycle completed",
			observability.Int("untracked_intents_count", len(report.UntrackedIntents)),
			observability.Int("duplicate_targets_count", len(report.DuplicateTargets)),
		)
		return nil
	},
		attribute.String("component", "selfservice.sweeper"),
	)
}
