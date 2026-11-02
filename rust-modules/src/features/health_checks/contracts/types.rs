use serde::Serialize;
use std::collections::HashMap;

#[derive(Serialize, Clone)]
pub struct HealthStatus {
    pub status: String,
    pub version: String,
    pub uptime_seconds: u64,
    pub timestamp: String,
}

#[derive(Serialize, Clone)]
pub struct ReadinessStatus {
    pub status: String,
    pub dependencies: HashMap<String, String>,
    pub timestamp: String,
}
