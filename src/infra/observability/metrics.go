/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PROMETHEUS METRICS REGISTRY & SSP / ROLLOUT TELEMETRY

 1. Role:
    - Registers strongly-typed Prometheus metrics for Self-Service Provisioning (ssp_*) and Progressive Rollout (platform_*).
    - Exposes the official promhttp.Handler() for Prometheus scraping on the dedicated metrics port (:9090) and admin port (/metrics).
 2. Invariants:
    - Uses github.com/prometheus/client_golang exclusively (zero hand-rolled exposition formatting).
    - Initializes zero-tolerance gauges and baseline label sets at startup so dashboards and alert rules evaluate cleanly.
    - Zero mid-function inline comments below this header.
*/
package observability

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type SSPMetrics struct {
	SweeperOrphanedDirty              prometheus.Gauge
	SweeperDuplicates                 prometheus.Gauge
	SweeperDanglingDNS                prometheus.Gauge
	SagaStalled                       prometheus.Gauge
	ObservedReadyNoHealthyReplicas    prometheus.Gauge
	RepoSecretScanViolations          prometheus.Counter
	RetentionUnauthorizedDeletions    prometheus.Counter
	StaleOutcomeRejections            prometheus.Counter
	IPCIDRAllocated                   prometheus.Gauge
	IPCIDRTotalCapacity               prometheus.Gauge
	TenantQuotaUnitsUsed              prometheus.Gauge
	TenantQuotaUnitsLimit             prometheus.Gauge
	SagaActiveCount                   *prometheus.GaugeVec
	ReservationsCount                 *prometheus.GaugeVec
	SagaStepExecutions                *prometheus.CounterVec
	SagaStepRetries                   *prometheus.CounterVec
	DownstreamRequests                *prometheus.CounterVec
	DownstreamDurationSeconds         *prometheus.HistogramVec
	MeshTrafficWeightActual           *prometheus.GaugeVec
	MeshTrafficWeightTarget           *prometheus.GaugeVec
	RolloutErrorRatePercent           *prometheus.GaugeVec
	RolloutP99LatencyMs               *prometheus.GaugeVec
	RolloutAbortsTotal                prometheus.Counter
	RolloutPretrafficFailuresTotal    prometheus.Counter
}

var (
	metricsOnce sync.Once
	sspMetrics  *SSPMetrics
)

func InitMetrics() *SSPMetrics {
	metricsOnce.Do(func() {
		m := &SSPMetrics{
			SweeperOrphanedDirty: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_sweeper_orphaned_dirty_count",
				Help: "Number of untracked or orphaned dirty provisioning intents detected by the sweeper.",
			}),
			SweeperDuplicates: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_sweeper_duplicates_count",
				Help: "Number of duplicate provisioning targets detected by the sweeper.",
			}),
			SweeperDanglingDNS: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_sweeper_dangling_dns_count",
				Help: "Number of dangling DNS cooldown records detected by the sweeper.",
			}),
			SagaStalled: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_saga_stalled_count",
				Help: "Number of stalled provisioning sagas.",
			}),
			ObservedReadyNoHealthyReplicas: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_observed_ready_no_healthy_replicas",
				Help: "Number of workloads marked ready without healthy replicas.",
			}),
			RepoSecretScanViolations: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "ssp_repo_secret_scan_violations_total",
				Help: "Total number of repository secret scan violations blocked during provisioning.",
			}),
			RetentionUnauthorizedDeletions: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "ssp_retention_unauthorized_deletion_attempts_total",
				Help: "Total number of unauthorized retention deletion attempts blocked.",
			}),
			StaleOutcomeRejections: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "ssp_stale_outcome_rejections_total",
				Help: "Total number of stale saga outcome rejections via fencing tokens.",
			}),
			IPCIDRAllocated: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_ip_cidr_allocated_count",
				Help: "Number of IP CIDR blocks currently allocated.",
			}),
			IPCIDRTotalCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_ip_cidr_total_capacity",
				Help: "Total IP CIDR block capacity available.",
			}),
			TenantQuotaUnitsUsed: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_tenant_quota_units_used",
				Help: "Total tenant capacity quota units currently used.",
			}),
			TenantQuotaUnitsLimit: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ssp_tenant_quota_units_limit",
				Help: "Total tenant capacity quota units limit.",
			}),
			SagaActiveCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "ssp_saga_active_count",
				Help: "Active provisioning sagas grouped by state.",
			}, []string{"state"}),
			ReservationsCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "ssp_reservations_count",
				Help: "Provisioning reservations grouped by status.",
			}, []string{"status"}),
			SagaStepExecutions: prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "ssp_saga_step_executions_total",
				Help: "Total provisioning saga step executions.",
			}, []string{"step"}),
			SagaStepRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "ssp_saga_step_retries_total",
				Help: "Total provisioning saga step retries.",
			}, []string{"step_type"}),
			DownstreamRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "ssp_downstream_requests_total",
				Help: "Total downstream provisioning client requests.",
			}, []string{"downstream", "status"}),
			DownstreamDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Name:    "ssp_downstream_duration_seconds",
				Help:    "Duration of downstream provisioning client requests in seconds.",
				Buckets: prometheus.DefBuckets,
			}, []string{"downstream"}),
			MeshTrafficWeightActual: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "platform_mesh_traffic_weight_actual",
				Help: "Actual canary mesh traffic weight percentage.",
			}, []string{"workload_id"}),
			MeshTrafficWeightTarget: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "platform_mesh_traffic_weight_target",
				Help: "Target canary mesh traffic weight percentage.",
			}, []string{"workload_id"}),
			RolloutErrorRatePercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "platform_rollout_error_rate_percent",
				Help: "Canary rollout error rate percentage.",
			}, []string{"workload_id"}),
			RolloutP99LatencyMs: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "platform_rollout_p99_latency_ms",
				Help: "Canary rollout p99 latency in milliseconds.",
			}, []string{"workload_id"}),
			RolloutAbortsTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "platform_rollout_aborts_total",
				Help: "Total canary rollout aborts.",
			}),
			RolloutPretrafficFailuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "platform_rollout_pretraffic_failures_total",
				Help: "Total canary rollout pre-traffic gate failures.",
			}),
		}

		prometheus.MustRegister(
			m.SweeperOrphanedDirty,
			m.SweeperDuplicates,
			m.SweeperDanglingDNS,
			m.SagaStalled,
			m.ObservedReadyNoHealthyReplicas,
			m.RepoSecretScanViolations,
			m.RetentionUnauthorizedDeletions,
			m.StaleOutcomeRejections,
			m.IPCIDRAllocated,
			m.IPCIDRTotalCapacity,
			m.TenantQuotaUnitsUsed,
			m.TenantQuotaUnitsLimit,
			m.SagaActiveCount,
			m.ReservationsCount,
			m.SagaStepExecutions,
			m.SagaStepRetries,
			m.DownstreamRequests,
			m.DownstreamDurationSeconds,
			m.MeshTrafficWeightActual,
			m.MeshTrafficWeightTarget,
			m.RolloutErrorRatePercent,
			m.RolloutP99LatencyMs,
			m.RolloutAbortsTotal,
			m.RolloutPretrafficFailuresTotal,
		)

		initializeBaselineMetrics(m)
		sspMetrics = m
	})
	return sspMetrics
}

func GetSSPMetrics() *SSPMetrics {
	return InitMetrics()
}

func MetricsHandler() http.Handler {
	InitMetrics()
	return promhttp.Handler()
}

func initializeBaselineMetrics(m *SSPMetrics) {
	m.IPCIDRTotalCapacity.Set(1024)
	m.TenantQuotaUnitsLimit.Set(100)

	for _, state := range []string{"PENDING", "PROVISIONING", "VERIFYING", "ROLLING_BACK", "COMPLETED", "FAILED"} {
		m.SagaActiveCount.WithLabelValues(state).Set(0)
	}
	for _, status := range []string{"ACTIVE", "RELEASED", "EXPIRED"} {
		m.ReservationsCount.WithLabelValues(status).Set(0)
	}
	for _, step := range []string{"reserve", "provision", "verify", "commit"} {
		m.SagaStepExecutions.WithLabelValues(step).Add(0)
	}
	for _, stepType := range []string{"provision", "verify", "compensate"} {
		m.SagaStepRetries.WithLabelValues(stepType).Add(0)
	}
	for _, downstream := range []string{"cluster", "cloudresource"} {
		m.DownstreamRequests.WithLabelValues(downstream, "200").Add(0)
		m.DownstreamRequests.WithLabelValues(downstream, "500").Add(0)
		m.DownstreamDurationSeconds.WithLabelValues(downstream).Observe(0)
	}
}
