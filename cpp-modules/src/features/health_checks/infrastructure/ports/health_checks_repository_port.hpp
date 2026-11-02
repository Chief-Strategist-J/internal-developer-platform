#pragma once

#include "features/health_checks/contracts/health_checks_types.hpp"

namespace idp::features::health_checks::infrastructure::ports {

class HealthChecksRepositoryPort {
public:
    virtual ~HealthChecksRepositoryPort() = default;
    virtual contracts::HealthStatus getLiveness() = 0;
    virtual contracts::ReadinessStatus getReadiness() = 0;
};

}
