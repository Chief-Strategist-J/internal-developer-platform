#pragma once

#include <memory>
#include <optional>
#include <string>
#include "api/rest/v1/handlers/health_checks_handler.hpp"

namespace idp::api::rest::v1::routers {

class HealthChecksRouter {
public:
    explicit HealthChecksRouter(std::shared_ptr<handlers::HealthChecksHandler> handler)
        : handler_(std::move(handler)) {}

    std::optional<std::pair<int, std::string>> route(const std::string& method, const std::string& path, const std::string& traceId) {
        if (method == "GET" && (path == "/healthz" || path == "/livez")) {
            return handler_->handleLiveness(traceId);
        }
        if (method == "GET" && path == "/readyz") {
            return handler_->handleReadiness(traceId);
        }
        return std::nullopt;
    }

private:
    std::shared_ptr<handlers::HealthChecksHandler> handler_;
};

}
