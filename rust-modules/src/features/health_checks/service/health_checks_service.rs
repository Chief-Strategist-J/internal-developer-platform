use std::sync::Arc;

use crate::features::health_checks::contracts::types::{HealthStatus, ReadinessStatus};
use crate::features::health_checks::infrastructure::ports::health_checks_repository_port::HealthChecksRepositoryPort;
use crate::shared::errors::ApplicationError;

pub struct HealthChecksService {
    repository: Arc<dyn HealthChecksRepositoryPort>,
}

impl HealthChecksService {
    pub fn new(repository: Arc<dyn HealthChecksRepositoryPort>) -> Self {
        Self { repository }
    }

    pub fn check_liveness(&self) -> Result<HealthStatus, ApplicationError> {
        self.repository.get_liveness()
    }

    pub fn check_readiness(&self) -> Result<ReadinessStatus, ApplicationError> {
        self.repository.get_readiness()
    }
}
