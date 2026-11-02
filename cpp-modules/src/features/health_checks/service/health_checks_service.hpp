#pragma once

#include <memory>
#include "features/health_checks/contracts/health_checks_types.hpp"
#include "features/health_checks/infrastructure/ports/health_checks_repository_port.hpp"

namespace idp::features::health_checks::service {

class HealthChecksService {
public:
    explicit HealthChecksService(std::shared_ptr<infrastructure::ports::HealthChecksRepositoryPort> repository)
        : repository_(std::move(repository)) {}

    contracts::HealthStatus checkLiveness() {
        return repository_->getLiveness();
    }

    contracts::ReadinessStatus checkReadiness() {
        return repository_->getReadiness();
    }

private:
    std::shared_ptr<infrastructure::ports::HealthChecksRepositoryPort> repository_;
};

}
