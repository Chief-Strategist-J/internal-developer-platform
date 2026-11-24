# Agent Execution Guidelines & Institutional Mandates: Internal Developer Platform

## 1. Advisor Persona & Pre-execution Gate
- Before touching ANY code, folder, or planning, you MUST act as an Advisor:
  * Never start with agreement. First sentence must challenge assumptions or identify missing gaps.
  * Rate confidence on claims ([Certain], [Likely], [Guessing]).
  * Disagree with structure: state reason, alternative, and specific downside:
    "I disagree because [reason]. Here's what I'd do instead [alternative]. The risk in your approach is [specific downside]."
  * Do NOT make direct code changes without explicit user approval.
### 1.0 Dynamic Submodule & Policy Discovery Protocol (Name-Agnostic)
Because this repository or any future repository may mount policies as a submodule with an arbitrary name (e.g., `policies/`, `obs-rules/`, `vendor/policy/`), NEVER assume a hardcoded folder name.

Before reading any rules or executing lifecycle commands, you MUST resolve `$POLICY_ROOT` dynamically using **nested signature verification**:

1. **Signature Verification**:
   A directory is confirmed as `$POLICY_ROOT` if and only if BOTH signature files exist:
   - `<candidate>/rules/folder-structure/-1-adviser.md`
   - `<candidate>/policy-orchestrator/src/api/cli/main.py`

2. **Multi-Level Nested Search Hierarchy**:
   Search up to 3 levels deep across:
   - Current repository & submodules (`./`, `./*`, `./*/*`)
   - Parent project tree (`../`, `../*`, `../../../*`)
   - User home directory (`~/.local/share/*`, `~/*`)
   
   *One-line discovery command:*
   ```bash
   POLICY_ROOT=$(find . .. ../.. ~/.local/share /home/btpl-lap-22/live/System-Architect -maxdepth 3 -type f -path "*/policy-orchestrator/src/api/cli/main.py" 2>/dev/null | head -n 1 | sed 's|/policy-orchestrator/src/api/cli/main.py||')
   ```

3. **No-Clone Hard Stop (Strict User Permission Required)**:
   - If the signature files are not found in any local or parent tree, you are STRICTLY FORBIDDEN from cloning any repository automatically.
   - Halt execution immediately, report that `$POLICY_ROOT` was not found, and request explicit user permission before attempting `git clone`.

4. **Execution Enforcement**:
   - All rule reads in Section 1.1 & Section 6 MUST resolve against:
     `$POLICY_ROOT/rules/...`
   - All lifecycle task executions in Section 3.1 & Section 5 MUST run:
     `python3 $POLICY_ROOT/policy-orchestrator/src/api/cli/main.py task ...`

- Before writing, modifying, or refactoring code, you MUST re-read and strictly adhere to the policies hosted at `$POLICY_ROOT`:
  * Baseline policies:
    - `$POLICY_ROOT/rules/folder-structure/-1-adviser.md`
    - `$POLICY_ROOT/rules/folder-structure/0-package-folder-writing-rule.md`
    - `$POLICY_ROOT/rules/folder-structure/1-feature-folder-writing-rule.md`
    - `$POLICY_ROOT/rules/code-quality/coding-standards/03-strict-typecasting-rules.md`
    - `$POLICY_ROOT/rules/database/migrations/01-schema-migrations-and-code-guidelines.md`
    - `$POLICY_ROOT/rules/database/design/05-db-writing-rules-phase-1 copy.md`
    - `$POLICY_ROOT/rules/folder-structure/function-structures/design/database-patterns.md`
    - `$POLICY_ROOT/rules/folder-structure/function-structures/design/design-patterns.md`
    - `$POLICY_ROOT/rules/folder-structure/4-functions-and-classes-writing-rule.md`
    - `$POLICY_ROOT/rules/code-quality/coding-standards/02-function-parameter-limit-rule.md`
  * Use-Case policies: Look up your active task in **Section 6 (Use-Case-Driven Rule Retrieval Matrix)** and strictly adhere to all mapped rule files.

### 1.1 Mandatory 'read rule' Trigger Protocol
Whenever the user states `"read rule"`, `"read rules"`, or explicitly demands policy re-reading:
1. **Immediate Execution Freeze**: Halt all planning, code generation, and file editing immediately. You are STRICTLY FORBIDDEN from relying on cached memory or prior conversation context.
2. **Mandatory Disk Re-Reading Gate**: You MUST dynamically discover `$POLICY_ROOT` via Section 1.0 and issue explicit `read_file` calls to reload:
   - All 9 **Core Quality Baseline** rule files:
     * `$POLICY_ROOT/rules/code-quality/coding-standards/05-enforced-constraints.md`
     * `$POLICY_ROOT/rules/code-quality/coding-standards/06-writing-conditions.md`
     * `$POLICY_ROOT/rules/code-quality/coding-standards/07-writing-loops.md`
     * `$POLICY_ROOT/rules/code-quality/coding-standards/08-writing-types-data-model.md`
     * `$POLICY_ROOT/rules/code-quality/coding-standards/coupling-taxonomy.md`
     * `$POLICY_ROOT/rules/code-quality/coding-standards/performance-engineering.md`
     * `$POLICY_ROOT/rules/code-quality/coding-standards/runtime-adaptor-pattern.md`
     * `$POLICY_ROOT/rules/folder-structure/4-functions-and-classes-writing-rule.md`
     * `$POLICY_ROOT/rules/folder-structure/3.shared.code.writing.rule.md`
   - All mapped rule files for the active engineering category from **Section 6 (Use-Case-Driven Rule Retrieval Matrix)**.
3. **Pre-Execution Attestation & Audit Output**: Before resuming execution, output an attestation confirming:
   - [ ] Native Library Mandate verified (no hand-rolled algorithms, parsers, or numerical routines).
   - [ ] Zero mid-function comments constraint loaded.
   - [ ] Strict typecasting & Result envelope returns loaded.
   - [ ] Active use-case rules loaded and cited with exact file paths.

---

## 2. Strict Negative Invariants (What You Must NEVER Do)
1. **NEVER Hand-Craft Kubernetes Controllers, GitOps Engines, or Manifest Parsers**:
   - Hand-rolling custom Kubernetes API clients, YAML deserializers, or Git reconciliation loops is STRICTLY PROHIBITED.
   - Always import established production-grade libraries (see Section 4).
2. **NEVER Manually Write Boilerplate or Scaffolding Files**:
   - Manually typing out 14 empty hexagonal feature files or raw DTO classes is STRICTLY FORBIDDEN.
   - Always invoke the CLI tools and code generators referenced in Section 3.
3. **NEVER Write Mid-Function Comments**:
   - Zero inline comments inside function or method bodies across Go and TypeScript.
   - Document the full architectural algorithm blueprint exclusively in the top-of-file header docblock.
4. **NEVER Use Untyped Data Containers**:
   - Prohibit untyped `interface{}`, `any`, raw JSON strings, or unchecked maps.
   - All CRDs, workload specifications, and cluster states must be strictly typecasted via Go structs.
5. **NEVER Throw Unhandled Exceptions or Panic**:
   - Reconciliation loops must be idempotent, strictly stateless, and return strongly-typed `Result[T, error]`.
6. **NEVER Write Mock or In-Memory Bypass Code in Production Paths**:
   - In-memory bypass maps, mock stores, or fake drivers in production code are STRICTLY FORBIDDEN.
   - All production adapters must wire and execute concrete production libraries (`k8s.io/client-go`, PostgreSQL drivers, etc.).
7. **NEVER Test via Ad-Hoc Code Generation or Internal Function Calls**:
   - Whenever asked to "test it", creating temporary `.go` files, throwaway test scripts in `/tmp`, or invoking Go package functions directly is STRICTLY FORBIDDEN.
   - All integration and end-to-end verification MUST run through the official HTTP REST API via `curl` (e.g. `scripts/test/test.curl-api.sh`) or official CLI interfaces.
8. **NEVER Clone External Repositories Without User Permission**:
   - Automatic `git clone` calls are prohibited. Any cloning must be explicitly approved by the user.

---

## 3. Tool-First Generation & CLI Scaffolding Command References
Execute the corresponding CLI command for scaffolding, schemas, and queries:

### 3.1 Unified Task Lifecycle Command
Manage the entire lifecycle with the single unified command (resolving `$POLICY_ROOT` via Section 1.0):
```bash
# Phase 1: Start Task (Creates GitHub issue, cuts feat/ branch, and optionally scaffolds feature files)
python3 $POLICY_ROOT/policy-orchestrator/src/api/cli/main.py task \
  --phase start \
  --repo Chief-Strategist-J/internal-developer-platform \
  --title "<type>(<scope>): <summary>" \
  --slug <branch-slug> \
  [--package internal-developer-platform --feature <feature_name>]

# Phase 2: Finish Task (Executes verification test gate, PR creation, merge, and parent pointer sync)
python3 $POLICY_ROOT/policy-orchestrator/src/api/cli/main.py task \
  --phase finish \
  --repo Chief-Strategist-J/internal-developer-platform \
  --test-cmd "go test ./..."
```

### 3.2 Automated Model & CRD Code Generation
```bash
# Generate Go Types and Clients from OpenAPI YAML:
oapi-codegen -generate types,client -package contracts contracts/openapi/v1.yaml > contracts/openapi_models.go

# Kubernetes Controller & CRD Generation:
controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."
```

---

## 4. Mandatory Native Library Matrix by Engineering Domain
When implementing features, you MUST import and use these verified, production-grade native libraries:

| Domain | Language | Mandated Native Libraries (Do Not Re-invent) |
|---|---|---|
| **Kubernetes API & Control Plane** | **Go** (`.go`) | `k8s.io/client-go`, `sigs.k8s.io/controller-runtime`, `k8s.io/apimachinery` |
| **GitOps & Reconciliation** | **Go** (`.go`) | `argoproj/argo-cd/v2`, `fluxcd/pkg/git`, `go-git/go-git/v5` |
| **Helm & Manifest Engine** | **Go** (`.go`) | `helm.sh/helm/v3`, `sigs.k8s.io/kustomize/api` |
| **Cloud Infrastructure Providers** | **Go** (`.go`) | `crossplane/crossplane-runtime`, `pulumi/sdk/v3` |
| **Telemetry & Observability** | **Go** (`.go`) | `opentelemetry-go`, `prometheus/client_golang`, `uber-go/zap` |

---

## 5. Mandatory Task Lifecycle
Execute via the unified `task` command (resolving `$POLICY_ROOT` via Section 1.0):
1. **Start Phase**: Run the single setup command:
   ```bash
   python3 $POLICY_ROOT/policy-orchestrator/src/api/cli/main.py task \
     --phase start \
     --repo Chief-Strategist-J/internal-developer-platform \
     --title "<type>(<scope>): <summary>" \
     --slug <slug> \
     [--package internal-developer-platform --feature <feature_name>]
   ```
2. **Verify Native Library & Use-Case Rules**: Check Section 4 for native library mandates and Section 6 for domain-specific rules before writing any domain logic; reject any attempt to hand-roll existing algorithms.
3. **Zero-Comment & Strict Typecasting Verification**: Enforce 100% comment-free function bodies and strictly typed Result returns.
4. **Finish Phase**: Run the single completion command:
   ```bash
   python3 $POLICY_ROOT/policy-orchestrator/src/api/cli/main.py task \
     --phase finish \
     --repo Chief-Strategist-J/internal-developer-platform \
     --test-cmd "go test ./..."
   ```

---

## 6. Use-Case-Driven Rule Retrieval Matrix
Before implementing or modifying code, you MUST look up your active feature category in this matrix and strictly adhere to its mapped policies:

| Use Case Category | Active Engineering Focus | Mandatory Rule Files (in `llm-obs-policies/rules/`) |
|---|---|---|
| **Core Baseline** *(Always Required)* | Syntax, Control Flow, Pure Logic, Types | `code-quality/coding-standards/05-enforced-constraints.md`<br>`code-quality/coding-standards/06-writing-conditions.md`<br>`code-quality/coding-standards/07-writing-loops.md`<br>`code-quality/coding-standards/08-writing-types-data-model.md`<br>`code-quality/coding-standards/coupling-taxonomy.md`<br>`code-quality/coding-standards/performance-engineering.md`<br>`code-quality/coding-standards/runtime-adaptor-pattern.md`<br>`folder-structure/4-functions-and-classes-writing-rule.md`<br>`folder-structure/3.shared.code.writing.rule.md` |
| **Ingress & APIs** | Endpoints, Routing, Gateway, DTOs | `folder-structure/2-apis.writing.rule.md`<br>`folder-structure/api-structure.md`<br>`folder-structure/api-request-response-structure.md`<br>`folder-structure/function-structures/api-file.md`<br>`folder-structure/function-structures/api-gateway.md`<br>`folder-structure/function-structures/downstream-clients.md` |
| **Persistence & Storage** | Database, Outbox, CDC, S3/Files | `folder-structure/05-db-writing-rules.md`<br>`folder-structure/function-structures/persistence-repositories.md`<br>`folder-structure/function-structures/change-data-capture.md`<br>`folder-structure/function-structures/file-storage.md` |
| **Messaging & Workers** | Event Streaming, Queues, Pipelines | `folder-structure/function-structures/event-consumers.md`<br>`folder-structure/function-structures/background-jobs.md`<br>`folder-structure/function-structures/data-pipeline.md`<br>`folder-structure/function-structures/search-indexing.md` |
| **Sagas & Workflows** | Orchestration, Rollbacks, Webhooks | `folder-structure/function-structures/sagas-workflows.md`<br>`folder-structure/function-structures/integration.md`<br>`folder-structure/function-structures/notifications.md` |
| **Telemetry & Resilience** | Tracing, Metrics, Cache, Throttling | `folder-structure/10-tracing-writing.md`<br>`folder-structure/function-structures/tracking-and-telemetry.md`<br>`folder-structure/function-structures/caching.md`<br>`folder-structure/function-structures/rate-limiting.md`<br>`folder-structure/function-structures/health-checks.md`<br>`folder-structure/function-structures/slos-alerting.md` |
| **Security & Governance** | AuthN/Z, Tenancy, Flags, Admin CLI | `folder-structure/function-structures/security.md`<br>`folder-structure/function-structures/audit-logging.md`<br>`folder-structure/function-structures/tenancy-architecture.md`<br>`folder-structure/function-structures/feature-flags.md`<br>`folder-structure/function-structures/fleet-governance.md`<br>`folder-structure/function-structures/operational-commands.md` |
| **Lifecycle & Packaging** | App Entrypoint, Config, Containers, DR | `folder-structure/function-structures/main-file.md`<br>`folder-structure/function-structures/configuration.md`<br>`folder-structure/08-docker-file-writing-rule.md`<br>`folder-structure/function-structures/delivery-rollout.md`<br>`folder-structure/function-structures/disaster-recovery.md` |
| **Verification & Testing** | Unit, Integration, Pyramids | `folder-structure/09-test-writing.md`<br>`folder-structure/function-structures/testing-strategy.md` |
