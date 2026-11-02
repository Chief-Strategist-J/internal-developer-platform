use crate::features::health_checks::contracts::types::{HealthStatus, ReadinessStatus};
use crate::shared::errors::ApplicationError;

pub trait HealthChecksRepositoryPort: Send + Sync {
    fn get_liveness(&self) -> Result<HealthStatus, ApplicationError>;
    fn get_readiness(&self) -> Result<ReadinessStatus, ApplicationError>;
}
