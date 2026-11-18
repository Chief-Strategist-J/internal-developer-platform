-- migration:      0006
-- description:    create policy tables with 05-db-writing-rules compliance
-- author:         platform-security
-- date:           2026-11-18T10:35:00Z
-- depends_on:     0005
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         initial schema for policy-as-code feature adhering strictly to 05-db-writing-rules

CREATE TABLE IF NOT EXISTS policy_lifecycle_states (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO policy_lifecycle_states (code, description) VALUES
    ('DRAFT', 'Policy drafted, not enforcing'),
    ('CI_ONLY', 'Evaluated only in pull request CI checks'),
    ('AUDIT', 'Audit mode, violations recorded without denial'),
    ('WARN', 'Warning mode with admission audit headers'),
    ('DENY', 'Active denial mode at admission/enforcement point'),
    ('ROLLED_BACK', 'Reverted policy state'),
    ('RETIRED', 'Retired policy replaced by newer control')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS policy_state_transitions (
    from_status TEXT NOT NULL REFERENCES policy_lifecycle_states(code),
    to_status TEXT NOT NULL REFERENCES policy_lifecycle_states(code),
    PRIMARY KEY (from_status, to_status)
);

INSERT INTO policy_state_transitions (from_status, to_status) VALUES
    ('DRAFT', 'CI_ONLY'),
    ('CI_ONLY', 'AUDIT'),
    ('AUDIT', 'WARN'),
    ('WARN', 'DENY'),
    ('DRAFT', 'ROLLED_BACK'),
    ('CI_ONLY', 'ROLLED_BACK'),
    ('AUDIT', 'ROLLED_BACK'),
    ('WARN', 'ROLLED_BACK'),
    ('DENY', 'ROLLED_BACK'),
    ('ROLLED_BACK', 'DRAFT'),
    ('DENY', 'RETIRED')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS policy_exception_status (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO policy_exception_status (code, description) VALUES
    ('PENDING', 'Exception pending security review'),
    ('ACTIVE', 'Active approved workload exemption'),
    ('REVOKED', 'Revoked exemption'),
    ('EXPIRED', 'Expired exemption')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS policies (
    id UUID NOT NULL,
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    correlation_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enforcement_point TEXT NOT NULL,
    severity TEXT NOT NULL,
    lifecycle_state TEXT NOT NULL,
    rule_expression TEXT NOT NULL,
    exceptable BOOLEAN NOT NULL DEFAULT true,
    detection_pair TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE NULL,

    CONSTRAINT policies_tenant_id_id_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT policies_public_id_uk UNIQUE (public_id),
    CONSTRAINT policies_tenant_source_ext_uk UNIQUE (tenant_id, source, external_id),
    CONSTRAINT policies_source_ck CHECK (source IN ('GIT', 'API', 'SYSTEM')),
    CONSTRAINT policies_name_ck CHECK (name = btrim(name) AND name <> '' AND length(name) <= 256),
    CONSTRAINT policies_description_ck CHECK (description = btrim(description)),
    CONSTRAINT policies_enforcement_point_ck CHECK (enforcement_point IN ('CEL', 'PSA', 'PC', 'BA', 'ORG', 'CI', 'ACD')),
    CONSTRAINT policies_severity_ck CHECK (severity IN ('CRITICAL', 'HIGH', 'MEDIUM', 'ADVISORY')),
    CONSTRAINT policies_rule_expression_ck CHECK (rule_expression = btrim(rule_expression) AND rule_expression <> ''),
    CONSTRAINT policies_detection_pair_ck CHECK (detection_pair = btrim(detection_pair) AND detection_pair <> ''),
    CONSTRAINT policies_external_id_ck CHECK (external_id = btrim(external_id) AND external_id <> '' AND length(external_id) <= 128),
    CONSTRAINT policies_lifecycle_state_fk FOREIGN KEY (lifecycle_state) REFERENCES policy_lifecycle_states(code) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_policies_lifecycle ON policies (tenant_id, lifecycle_state);
CREATE INDEX IF NOT EXISTS idx_policies_enforcement ON policies (tenant_id, enforcement_point);

CREATE TABLE IF NOT EXISTS policy_exceptions (
    id UUID NOT NULL,
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    policy_id UUID NOT NULL,
    workload_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    approver_id TEXT NOT NULL,
    justification TEXT NOT NULL,
    status TEXT NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    correlation_id UUID NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE NULL,

    CONSTRAINT policy_exceptions_tenant_id_id_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT policy_exceptions_public_id_uk UNIQUE (public_id),
    CONSTRAINT policy_exceptions_policy_fk FOREIGN KEY (tenant_id, policy_id) REFERENCES policies(tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT policy_exceptions_status_fk FOREIGN KEY (status) REFERENCES policy_exception_status(code) ON DELETE RESTRICT,
    CONSTRAINT policy_exceptions_workload_ck CHECK (workload_id = btrim(workload_id) AND workload_id <> '' AND length(workload_id) <= 256),
    CONSTRAINT policy_exceptions_namespace_ck CHECK (namespace = btrim(namespace) AND namespace <> '' AND length(namespace) <= 63),
    CONSTRAINT policy_exceptions_approver_ck CHECK (approver_id = btrim(approver_id) AND approver_id <> ''),
    CONSTRAINT policy_exceptions_justification_ck CHECK (length(btrim(justification)) >= 10),
    CONSTRAINT policy_exceptions_expires_ck CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS idx_policy_exceptions_policy ON policy_exceptions (tenant_id, policy_id);
CREATE INDEX IF NOT EXISTS idx_policy_exceptions_status ON policy_exceptions (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_policy_exceptions_workload ON policy_exceptions (tenant_id, namespace, workload_id);
