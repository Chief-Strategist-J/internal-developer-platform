use std::sync::Arc;

use crate::api::rest::v1::handlers::health_checks_handler::HealthChecksHandler;

pub struct HealthChecksRouter {
    handler: Arc<HealthChecksHandler>,
}

impl HealthChecksRouter {
    pub fn new(handler: Arc<HealthChecksHandler>) -> Self {
        Self { handler }
    }

    pub fn route(&self, method: &str, path: &str, trace_id: &str) -> Option<(u16, String)> {
        if method == "GET" && (path == "/healthz" || path == "/livez") {
            return Some(self.handler.handle_liveness(trace_id));
        }
        if method == "GET" && path == "/readyz" {
            return Some(self.handler.handle_readiness(trace_id));
        }
        None
    }
}
