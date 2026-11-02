use chrono::Utc;
use std::sync::Arc;

use crate::features::health_checks::HealthChecksService;
use crate::shared::types::{ApiErrorEnvelope, ApiMetaEnvelope, ApiResponseEnvelope};

pub struct HealthChecksHandler {
    service: Arc<HealthChecksService>,
}

impl HealthChecksHandler {
    pub fn new(service: Arc<HealthChecksService>) -> Self {
        Self { service }
    }

    pub fn handle_liveness(&self, trace_id: &str) -> (u16, String) {
        match self.service.check_liveness() {
            Ok(data) => {
                let envelope = ApiResponseEnvelope {
                    success: true,
                    status_code: 200,
                    data: Some(data),
                    error: None,
                    meta: ApiMetaEnvelope {
                        timestamp: Utc::now().to_rfc3339(),
                        trace_id: trace_id.to_string(),
                    },
                };
                (200, serde_json::to_string(&envelope).unwrap_or_default())
            }
            Err(err) => {
                let envelope: ApiResponseEnvelope<String> = ApiResponseEnvelope {
                    success: false,
                    status_code: err.status_code,
                    data: None,
                    error: Some(ApiErrorEnvelope {
                        code: err.code,
                        message: err.message,
                    }),
                    meta: ApiMetaEnvelope {
                        timestamp: Utc::now().to_rfc3339(),
                        trace_id: trace_id.to_string(),
                    },
                };
                (err.status_code, serde_json::to_string(&envelope).unwrap_or_default())
            }
        }
    }

    pub fn handle_readiness(&self, trace_id: &str) -> (u16, String) {
        match self.service.check_readiness() {
            Ok(data) => {
                let status_code = if data.status == "READY" { 200 } else { 503 };
                let envelope = ApiResponseEnvelope {
                    success: data.status == "READY",
                    status_code,
                    data: Some(data),
                    error: None,
                    meta: ApiMetaEnvelope {
                        timestamp: Utc::now().to_rfc3339(),
                        trace_id: trace_id.to_string(),
                    },
                };
                (status_code, serde_json::to_string(&envelope).unwrap_or_default())
            }
            Err(err) => {
                let envelope: ApiResponseEnvelope<String> = ApiResponseEnvelope {
                    success: false,
                    status_code: 503,
                    data: None,
                    error: Some(ApiErrorEnvelope {
                        code: err.code,
                        message: err.message,
                    }),
                    meta: ApiMetaEnvelope {
                        timestamp: Utc::now().to_rfc3339(),
                        trace_id: trace_id.to_string(),
                    },
                };
                (503, serde_json::to_string(&envelope).unwrap_or_default())
            }
        }
    }
}
