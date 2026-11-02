# SLO Management — Business Logic Decision Trees

## 1. Architectural Overview & Error Budget Decision Tree

```
SLO MANAGEMENT ENGINE (SLI MEASUREMENT & ERROR BUDGET GOVERNANCE DECISION TREE)
│
├── 1. SLI SPECIFICATION & WINDOW INGESTION TIER [P0]
│   └── Real-World: Service declares SLO: "99.9% of HTTP requests return status < 500 over rolling 30-day window"
│       └── Trigger: Inbound SLI Evaluation Loop (Every 60 seconds)
│           ├── Action: Compute Service Level Indicator (SLI) via Prometheus PromQL:
│           │   └── `SLI = sum(rate(http_requests_total{status!~"5.."}[30d])) / sum(rate(http_requests_total[30d]))`
│           ├── Calculation: Error Budget Status
│           │   ├── Target Availability: 99.9% (Allowable Error Rate: 0.1% = 0.001)
│           │   ├── Allowable Errors in Window = Total Requests * 0.001
│           │   ├── Consumed Errors = Actual HTTP 5xx Count
│           │   └── Remaining Error Budget % = `1.0 - (Consumed Errors / Allowable Errors)`
│           └── State: BUDGET_CALCULATED
│
├── 2. MULTI-WINDOW MULTI-BURN-RATE ALERTING TIER [P0]
│   └── Real-World: Sudden error spike burns 5% of monthly error budget in 1 hour (Google SRE Standard)
│       └── Evaluation: Burn Rate Calculations (Short-Window vs Long-Window)
│           ├── Burn Rate Scenario 1: Critical Emergency (14.4x Burn Rate)
│           │   ├── Condition: 2% of budget consumed in 1 hour (Long window: 1h, Short window: 5m)
│           │   ├── Action: Page On-Call immediately (SLA: 5m response)
│           │   └── State: CRITICAL_BUDGET_BURN
│           ├── Burn Rate Scenario 2: Severe Degradation (6x Burn Rate)
│           │   ├── Condition: 5% of budget consumed in 6 hours (Long window: 6h, Short window: 30m)
│           │   ├── Action: Page On-Call (SLA: 15m response)
│           │   └── State: SEVERE_BUDGET_BURN
│           └── Burn Rate Scenario 3: Slow Burn (1x Burn Rate - Budget on track to deplete by day 30)
│               └── Action: File Jira ticket for team sprint review (No off-hours page)
│           │
├── 3. ERROR BUDGET EXHAUSTION & POLICY GATEWAY TIER [P1]
│   └── Real-World: Team burns 100% of their monthly error budget; automated freeze on feature deployments
│       └── Evaluation: Error Budget Policy Gates
│           ├── Check: Is Remaining Error Budget % <= 0%?
│           │   ├── NO ──> Deployments Permitted (Green State)
│           │   └── YES (Error Budget Exhausted)
│           │       ├── State: ERROR_BUDGET_DEPLETED
│           │       ├── Action: Block Non-Emergency Feature Deployments in CI/CD Pipeline
│           │       ├── Action: Restrict Deployments to: Bugfixes, Reliability Improvements, Rollbacks
│           │       └── Check: Does Team Request Feature Deployment Exemption?
│           │           ├── YES ──> Require Engineering VP / Director Override Token
│           │           └── NO ──> Feature Freeze Enforced until budget recovers
│           │
└── 4. REPORTING & COMPLIANCE SCORECARDS TIER [P1]
    └── Real-World: Generating monthly executive reliability scorecards across all company microservices
        └── Trigger: End of Month Reporting Cron
            ├── Action: Aggregate SLO achievement % across all services
            ├── Classification:
            │   ├── Met Target (SLI >= SLO) ──> Green Rating
            │   └── Breached Target (SLI < SLO) ──> Red Rating (Mandate reliability sprint allocation)
            └── Publish to Executive Dashboard & Service Catalog
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Multi-Window Multi-Burn-Rate (Google SRE Alerting Standard)
- Real-Life Incident: A team alerted on raw error percentages; a 1-second spike at 3:00 AM woke up engineers, while a slow 0.5% leak went undetected for 2 weeks, silently burning the entire budget.
- Rationale: SLO alerting must look at both short and long windows simultaneously (Multi-Window Multi-Burn-Rate) to ensure alerts are mathematically guaranteed to represent real budget crises.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: MULTI-WINDOW BURN RATE (P0-01)
Trigger: Burn Rate Alert Evaluation
├── Check: Is Burn Rate > Threshold in 1-Hour Long Window?
│   ├── YES ──> Check: Is Burn Rate ALSO > Threshold in 5-Minute Short Window?
│   │   ├── YES ──> [FIRE ALERT: Mathematical proof that incident is active and budget is burning]
│   │   └── NO ──> [SUPPRESS: Spike has already ceased; do not page responder]
│   └── NO ──> Compliant
```

### Invariant P1-02: Automated Feature Freeze on Budget Depletion
- Real-Life Incident: A team with a broken service kept shipping new features, causing continuous customer downtime because product managers prioritized features over stability.
- Rationale: Error budgets create a shared contract between Product and SRE. When the budget is gone, feature work stops until reliability is restored.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: FEATURE FREEZE (P1-02)
Trigger: CI/CD Deployment Trigger for Production
├── Check: Service Error Budget <= 0%?
│   ├── YES ──> Check: Is Deployment tagged `type: reliability` or `type: hotfix`?
│   │   ├── YES ──> Allow Deployment
│   │   └── NO ──> [BLOCK: HTTP 403 Deployment Blocked — Error Budget Exhausted]
│   └── NO ──> Permit Deployment
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Low-Traffic Weekend Skews Rolling Error Budget
- Real-Life Incident: A B2B service with zero weekend traffic had 2 errors on Saturday, which calculated as 100% error rate and depleted the budget due to zero total volume.
- Decision Tree:
```
EDGE CASE 01: LOW VOLUME ERROR BUDGET SKEW
Trigger: Error Budget Calculation during low-traffic hours
├── Check: Total Request Volume in Window < Minimum Significance Floor?
│   ├── YES ──> [DEFER: Suppress burn rate alerts; require absolute error count threshold before paging]
│   └── NO ──> Standard Percentage Calculation
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
SLO MANAGEMENT ROADMAP
│
├── PHASE 0: SLI DEFINITION SCHEMA & PROMETHEUS CALCULATOR (P0 - WEEKS 1-3)
│   ├── Declarative SLO custom resource definition (CRD) via Sloth / Pyrra
│   ├── Automated generation of PromQL queries for availability and latency SLIs
│   └── Deliverable: Transparent rolling 30-day SLI dashboards for all services
│
├── PHASE 1: MULTI-WINDOW BURN-RATE ALERTS (P1 - WEEKS 4-6)
│   ├── Google SRE Multi-Window Multi-Burn-Rate alerting engine
│   ├── PagerDuty integration for 14.4x and 6x burn rate pages
│   └── Deliverable: 95% reduction in false-alarm alert noise for on-call engineers
│
├── PHASE 2: AUTOMATED CI/CD ERROR BUDGET GATES (P2 - WEEKS 7-9)
│   ├── CI/CD pipeline plugins querying error budget balance before deployment
│   ├── Automated deployment freeze policy enforcement
│   └── Deliverable: Automated contractual alignment between product and reliability
│
└── PHASE 3: COMPOSITE SLOS & USER JOURNEY MODELING (P3 - WEEKS 10-12)
    ├── Multi-service composite SLOs representing entire user checkout journeys
    └── Deliverable: True business-impact reliability governance
```
