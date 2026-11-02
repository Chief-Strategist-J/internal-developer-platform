use chrono::Utc;
use std::collections::HashMap;
use std::time::Instant;

use crate::features::health_checks::contracts::types::{HealthStatus, ReadinessStatus};
use crate::features::health_checks::infrastructure::ports::health_checks_repository_port::HealthChecksRepositoryPort;
use crate::shared::errors::ApplicationError;

pub struct HealthChecksRepositoryAdapter {
    start_instant: Instant,
}

impl HealthChecksRepositoryAdapter {
    pub fn new() -> Self {
        Self {
            start_instant: Instant::now(),
        }
    }
}

impl HealthChecksRepositoryPort for HealthChecksRepositoryAdapter {
    fn get_liveness(&self) -> Result<HealthStatus, ApplicationError> {
        let uptime_seconds = self.start_instant.elapsed().as_secs();
        Ok(HealthStatus {
            status: "UP".to_string(),
            version: "1.0.0".to_string(),
            uptime_seconds,
            timestamp: Utc::now().to_rfc3339(),
        })
    }

    fn get_readiness(&self) -> Result<ReadinessStatus, ApplicationError> {
        let mut dependencies = HashMap::new();
        dependencies.insert("database".to_string(), "UP".to_string());
        dependencies.insert("cache".to_string(), "UP".to_string());
        dependencies.insert("telemetry".to_string(), "UP".to_string());

        Ok(ReadinessStatus {
            status: "READY".to_string(),
            dependencies,
            timestamp: Utc::now().to_rfc3339(),
        })
    }
}
