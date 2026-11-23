package constants

const (
	HeaderTraceparent     = "traceparent"
	HeaderXRequestID      = "x-request-id"
	HeaderXCorrelationID  = "x-correlation-id"
	HeaderXCausationID    = "x-causation-id"
	HeaderXIdempotencyKey = "x-idempotency-key"
	HeaderXTenantID       = "x-tenant-id"
	HeaderXClientID       = "x-client-id"
	HeaderXUserID         = "x-user-id"
	HeaderXAPIVersion     = "x-api-version"
	HeaderIfMatch         = "if-match"
	HeaderETag            = "etag"
	HeaderRetryAfter      = "retry-after"
	HeaderAuthorization   = "Authorization"
	HeaderContentType     = "Content-Type"
	HeaderAccept          = "Accept"
)

const (
	ContentTypeJSON = "application/json"
)

const (
	EndpointAuthToken             = "/api/v1/auth/token"
	EndpointAuthMe                = "/api/v1/auth/me"
	EndpointAuthRevoke            = "/api/v1/auth/revoke"
	EndpointAuthEvaluate          = "/api/v1/auth/evaluate"
	EndpointAuthJITGrants         = "/api/v1/auth/jit/grants"
	EndpointAuthJITGrantsPrefix   = "/api/v1/auth/jit/grants/"
	EndpointReconciliations       = "/api/v1/reconciliations"
	EndpointReconciliationsPrefix = "/api/v1/reconciliations/"
	EndpointServices              = "/api/v1/services"
	EndpointWorkloads             = "/api/v1/workloads"
	EndpointWorkloadsPrefix       = "/api/v1/workloads/"
	EndpointRegimes               = "/api/v1/regimes"
	EndpointRegimesActive         = "/api/v1/regimes/active"
	EndpointAuthBindings          = "/api/v1/auth/bindings"
	EndpointAuthBindingsPrefix    = "/api/v1/auth/bindings/"
	EndpointClusters              = "/api/v1/clusters"
	EndpointClustersPrefix        = "/api/v1/clusters/"
	EndpointClustersPosture       = "/api/v1/clusters/posture"
	EndpointCloudResources        = "/api/v1/cloud-resources"
	EndpointCloudResourcesPrefix  = "/api/v1/cloud-resources/"
	EndpointCloudResourcesReconcile = "/api/v1/cloud-resources/reconcile"
	EndpointTimelinePrefix        = "/api/v1/workloads/timeline"
	EndpointPolicies              = "/api/v1/policies"
	EndpointPoliciesPrefix        = "/api/v1/policies/"
	EndpointPoliciesEvaluate            = "/api/v1/policies/admission/evaluate"
	EndpointPoliciesAuditScan           = "/api/v1/policies/audit-scan"
	EndpointPoliciesReconcileExceptions = "/api/v1/policies/exceptions/reconcile"
)

const (
	ErrCodeValidationFailed   = "VALIDATION_FAILED"
	ErrCodeConflict           = "CONFLICT"
	ErrCodeNotFound           = "RESOURCE_NOT_FOUND"
	ErrCodePreconditionFailed = "PRECONDITION_FAILED"
	ErrCodeInternalError      = "INTERNAL_SERVER_ERROR"
)
