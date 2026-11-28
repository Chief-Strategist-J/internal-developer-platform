/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING SERVERLESS & INGESTION-SIGNAL RULES (V.3 role 07, Invariant B, AS-INV-22, AS-INV-23)

1. Purpose:
   - AS-INV-22 (business-logic AS-19): HTTP scale-to-zero is never allowed in production, only behind an
     interceptor elsewhere, within its cold-start budget, and only when database reconnects are tolerated.
   - AS-INV-23 (business-logic AS-20): consumers scale on backlog, oldest-message age or lag with a CPU
     co-signal, dead-letter poison messages, keep production minimum at 1 or more, and for Kafka limit
     scaling to partitions with lag and decide invalid-offset behaviour explicitly.
2. Invariants:
   - Pure deterministic functions; zero IO, clocks, randomness or shared state.
   - Zero inline comments below this header.
*/

package rules

import (
	"fmt"

	"internal-developer-platform/src/features/autoscaling/internal/contracts"
)

func evaluateHTTPScaleToZero(plan contracts.ScalingPlan, serverless contracts.ServerlessScaling) []contracts.Violation {
	var out []contracts.Violation
	if !serverless.HTTPScaleToZero {
		return out
	}
	if plan.Environment == contracts.EnvironmentProduction {
		out = append(out, mapToViolation("AS-INV-22", "HTTP_SCALE_TO_ZERO_IN_PRODUCTION", contracts.ViolationSeverityPage,
			"HTTP scale-to-zero is not allowed in production"))
	}
	if !serverless.BehindInterceptor {
		out = append(out, mapToViolation("AS-INV-22", "HTTP_SCALE_TO_ZERO_NO_INTERCEPTOR", contracts.ViolationSeverityTicket,
			"a scaled-to-zero HTTP service fails requests unless an interceptor holds them"))
	}
	return out
}

func EvaluateScaleToZeroSafety(plan contracts.ScalingPlan, db contracts.DatabaseScalingPlan, serverless contracts.ServerlessScaling) []contracts.Violation {
	out := evaluateHTTPScaleToZero(plan, serverless)
	if !plan.ScaleToZero {
		return out
	}
	if serverless.ColdStartSeconds > serverless.ColdStartBudgetSeconds {
		out = append(out, mapToViolation("AS-INV-22", "COLD_START_EXCEEDS_BUDGET", contracts.ViolationSeverityTicket,
			fmt.Sprintf("cold start %ds exceeds the latency budget %ds", serverless.ColdStartSeconds, serverless.ColdStartBudgetSeconds)))
	}
	if db.TalksToDatabase && !serverless.ToleratesReconnectStorm {
		out = append(out, mapToViolation("AS-INV-22", "SCALE_TO_ZERO_RECONNECT_STORM", contracts.ViolationSeverityTicket,
			"scale-to-zero is rejected for services holding database connections that cannot tolerate a reconnect storm"))
	}
	return out
}

func isBacklogSignal(signal string) bool {
	switch signal {
	case contracts.IngestSignalBacklog, contracts.IngestSignalOldestAge, contracts.IngestSignalLag:
		return true
	}
	return false
}

func evaluateKafkaScaling(ingest contracts.IngestionScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if ingest.Backbone != contracts.BackboneKafka {
		return out
	}
	if !ingest.LimitToPartitionsWithLag {
		out = append(out, mapToViolation("AS-INV-23", "INGEST_NOT_LIMITED_TO_LAG", contracts.ViolationSeverityTicket,
			"limitToPartitionsWithLag must be set so one hot partition cannot scale the group to max"))
	}
	if !ingest.InvalidOffsetDecided {
		out = append(out, mapToViolation("AS-INV-23", "INGEST_OFFSET_BEHAVIOR_UNDECIDED", contracts.ViolationSeverityTicket,
			"scaleToZeroOnInvalidOffset must be decided explicitly"))
	}
	return out
}

func EvaluateIngestionSignal(plan contracts.ScalingPlan, ingest contracts.IngestionScalingPlan) []contracts.Violation {
	var out []contracts.Violation
	if ingest.Backbone == "" {
		return out
	}
	if !isBacklogSignal(ingest.Signal) {
		out = append(out, mapToViolation("AS-INV-23", "INGEST_SIGNAL_INVALID", contracts.ViolationSeverityTicket,
			fmt.Sprintf("consumers must scale on backlog, oldest message age or lag, not %q", ingest.Signal)))
	}
	if !ingest.DeadLetterConfigured {
		out = append(out, mapToViolation("AS-INV-23", "INGEST_NO_DEAD_LETTER", contracts.ViolationSeverityTicket,
			"poison messages must dead-letter so they cannot inflate the backlog forever"))
	}
	return append(out, evaluateKafkaScaling(ingest)...)
}
