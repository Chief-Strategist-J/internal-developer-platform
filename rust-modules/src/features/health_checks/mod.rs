pub(crate) mod api;
pub(crate) mod contracts;
pub(crate) mod infrastructure;
pub(crate) mod service;

use std::sync::Arc;

pub use api::rest::v1::health_checks_router::HealthChecksRouter;
use api::rest::v1::health_checks_handler::HealthChecksHandler;
use infrastructure::adapters::health_checks_repository_adapter::HealthChecksRepositoryAdapter;
use service::health_checks_service::HealthChecksService;

pub fn build_health_checks_router() -> Arc<HealthChecksRouter> {
    let repository = Arc::new(HealthChecksRepositoryAdapter::new());
    let service = Arc::new(HealthChecksService::new(repository));
    let handler = Arc::new(HealthChecksHandler::new(service));
    Arc::new(HealthChecksRouter::new(handler))
}
