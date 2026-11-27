#pragma once

#include <memory>

#include "features/health_checks/api/rest/v1/health_checks_rest_handler.hpp"
#include "features/health_checks/api/rest/v1/health_checks_router.hpp"
#include "features/health_checks/infrastructure/adapters/health_checks_repository_adapter.hpp"
#include "features/health_checks/service/health_checks_service.hpp"

namespace idp::features::health_checks {

inline std::shared_ptr<api::rest::v1::HealthChecksRouter> buildHealthChecksRouter() {
    auto repository = std::make_shared<infrastructure::adapters::HealthChecksRepositoryAdapter>();
    auto service = std::make_shared<service::HealthChecksService>(repository);
    auto handler = std::make_shared<api::rest::v1::HealthChecksHandler>(service);
    return std::make_shared<api::rest::v1::HealthChecksRouter>(handler);
}

}
