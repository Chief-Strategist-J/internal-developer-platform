#pragma once

#include <memory>
#include <sstream>
#include <string>
#include "features/health_checks/service/health_checks_service.hpp"

namespace idp::api::rest::v1::handlers {

class HealthChecksHandler {
public:
    explicit HealthChecksHandler(std::shared_ptr<features::health_checks::service::HealthChecksService> service)
        : service_(std::move(service)) {}

    std::pair<int, std::string> handleLiveness(const std::string& traceId) {
        try {
            const auto data = service_->checkLiveness();
            std::stringstream ss;
            ss << "{\"success\":true,\"statusCode\":200,\"data\":{"
               << "\"status\":\"" << data.status << "\","
               << "\"version\":\"" << data.version << "\","
               << "\"uptimeSeconds\":" << data.uptimeSeconds << ","
               << "\"timestamp\":\"" << data.timestamp << "\""
               << "},\"error\":null,\"meta\":{"
               << "\"timestamp\":\"" << data.timestamp << "\","
               << "\"traceId\":\"" << traceId << "\""
               << "}}";
            return {200, ss.str()};
        } catch (...) {
            return {500, "{\"success\":false,\"statusCode\":500,\"data\":null,\"error\":{\"code\":\"HEALTH_CHECK_FAILED\",\"message\":\"Liveness probe failed\"},\"meta\":{\"timestamp\":\"\",\"traceId\":\"" + traceId + "\"}}"};
        }
    }

    std::pair<int, std::string> handleReadiness(const std::string& traceId) {
        try {
            const auto data = service_->checkReadiness();
            const int statusCode = (data.status == "READY") ? 200 : 503;
            std::stringstream ss;
            ss << "{\"success\":" << (statusCode == 200 ? "true" : "false")
               << ",\"statusCode\":" << statusCode
               << ",\"data\":{\"status\":\"" << data.status << "\",\"dependencies\":{";

            bool first = true;
            for (const auto& [k, v] : data.dependencies) {
                if (!first) ss << ",";
                ss << "\"" << k << "\":\"" << v << "\"";
                first = false;
            }

            ss << "},\"timestamp\":\"" << data.timestamp << "\"},\"error\":null,\"meta\":{"
               << "\"timestamp\":\"" << data.timestamp << "\","
               << "\"traceId\":\"" << traceId << "\"}}";
            return {statusCode, ss.str()};
        } catch (...) {
            return {503, "{\"success\":false,\"statusCode\":503,\"data\":null,\"error\":{\"code\":\"SERVICE_UNAVAILABLE\",\"message\":\"Readiness probe failed\"},\"meta\":{\"timestamp\":\"\",\"traceId\":\"" + traceId + "\"}}"};
        }
    }

private:
    std::shared_ptr<features::health_checks::service::HealthChecksService> service_;
};

}
