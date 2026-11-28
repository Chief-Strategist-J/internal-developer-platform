/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES TYPES & CONTRACTS (V.3 role 01, P0-01..P0-08)

1. Purpose:
   - Declares domain entities, value types, and DTO envelopes for progressive delivery.
   - Enforces Data Contract v1.2 five-role identifiers and tenant isolation.
2. Invariants:
   - Strictly typed Go structs; zero untyped containers, interface{}, or any.
   - Zero inline comments below this header.
*/

package contracts

import (
	"time"
)

type StrategyType string

const (
	StrategyTypeCanary          StrategyType = "CANARY"
	StrategyTypeBlueGreen       StrategyType = "BLUE_GREEN"
	StrategyTypeProgressiveMesh StrategyType = "PROGRESSIVE_MESH"
)

type RolloutStatus string

const (
	RolloutStatusInitializing RolloutStatus = "INITIALIZING"
	RolloutStatusRoutingStep   RolloutStatus = "ROUTING_STEP"
	RolloutStatusAnalyzing     RolloutStatus = "ANALYZING"
	RolloutStatusPaused        RolloutStatus = "PAUSED"
	RolloutStatusPromoting     RolloutStatus = "PROMOTING"
	RolloutStatusCompleted     RolloutStatus = "COMPLETED"
	RolloutStatusAborting      RolloutStatus = "ABORTING"
	RolloutStatusAborted       RolloutStatus = "ABORTED"
)

type AnalysisVerdict string

const (
	AnalysisVerdictPass         AnalysisVerdict = "PASS"
	AnalysisVerdictFail         AnalysisVerdict = "FAIL"
	AnalysisVerdictInconclusive AnalysisVerdict = "INCONCLUSIVE"
)

type CanaryStep struct {
	StepIndex     int           `json:"stepIndex"`
	WeightPct     int           `json:"weightPct"`
	PauseDuration time.Duration `json:"pauseDuration"`
	Metrics       []string      `json:"metrics"`
}

type RolloutPlan struct {
	ID               string       `json:"id"`
	PublicID         string       `json:"publicId"`
	TenantID         string       `json:"tenantId"`
	Name             string       `json:"name"`
	StrategyType     StrategyType `json:"strategyType"`
	Steps            []CanaryStep `json:"steps"`
	FailureThreshold float64      `json:"failureThreshold"`
	CorrelationID    string       `json:"correlationId"`
	Version          int64        `json:"version"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
}

type RolloutExecution struct {
	ID               string        `json:"id"`
	PublicID         string        `json:"publicId"`
	TenantID         string        `json:"tenantId"`
	PlanID           string        `json:"planId"`
	WorkloadID       string        `json:"workloadId"`
	TargetImage      string        `json:"targetImage"`
	Status           RolloutStatus `json:"status"`
	CurrentStepIndex int           `json:"currentStepIndex"`
	CurrentWeightPct int           `json:"currentWeightPct"`
	ErrorMessage     string        `json:"errorMessage"`
	CorrelationID    string        `json:"correlationId"`
	Version          int64         `json:"version"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
}

type RolloutAnalysisRun struct {
	ID             string          `json:"id"`
	PublicID       string          `json:"publicId"`
	TenantID       string          `json:"tenantId"`
	ExecutionID    string          `json:"executionId"`
	MetricName     string          `json:"metricName"`
	Query          string          `json:"query"`
	MeasuredValue  float64         `json:"measuredValue"`
	ThresholdValue float64         `json:"thresholdValue"`
	Verdict        AnalysisVerdict `json:"verdict"`
	CorrelationID  string          `json:"correlationId"`
	CreatedAt      time.Time       `json:"createdAt"`
}

type StartRolloutInput struct {
	TenantID      string `json:"tenantId"`
	PlanID        string `json:"planId"`
	WorkloadID    string `json:"workloadId"`
	TargetImage   string `json:"targetImage"`
	CorrelationID string `json:"correlationId"`
}

type AbortRolloutInput struct {
	TenantID    string `json:"tenantId"`
	ExecutionID string `json:"executionId"`
	Reason      string `json:"reason"`
}

type ResumeRolloutInput struct {
	TenantID    string `json:"tenantId"`
	ExecutionID string `json:"executionId"`
}

type PodHealthReport struct {
	ReplicasDesired int  `json:"replicasDesired"`
	ReplicasReady   int  `json:"replicasReady"`
	SidecarInjected bool `json:"sidecarInjected"`
	SidecarHealthy  bool `json:"sidecarHealthy"`
}
