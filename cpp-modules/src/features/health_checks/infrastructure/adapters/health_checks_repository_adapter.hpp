#pragma once

#include <chrono>
#include <iomanip>
#include <sstream>
#include "features/health_checks/infrastructure/ports/health_checks_repository_port.hpp"

namespace idp::features::health_checks::infrastructure::adapters {

class HealthChecksRepositoryAdapter : public ports::HealthChecksRepositoryPort {
public:
    HealthChecksRepositoryAdapter() : startTime_(std::chrono::steady_clock::now()) {}

    contracts::HealthStatus getLiveness() override {
        const auto now = std::chrono::steady_clock::now();
        const auto uptime = std::chrono::duration_cast<std::chrono::seconds>(now - startTime_).count();
        return contracts::HealthStatus{
            .status = "UP",
            .version = "1.0.0",
            .uptimeSeconds = static_cast<std::uint64_t>(uptime),
            .timestamp = getIsoTimestamp()
        };
    }

    contracts::ReadinessStatus getReadiness() override {
        return contracts::ReadinessStatus{
            .status = "READY",
            .dependencies = {
                {"database", "UP"},
                {"cache", "UP"},
                {"telemetry", "UP"}
            },
            .timestamp = getIsoTimestamp()
        };
    }

private:
    std::chrono::steady_clock::time_point startTime_;

    static std::string getIsoTimestamp() {
        const auto now = std::chrono::system_clock::now();
        const auto inTimeT = std::chrono::system_clock::to_time_t(now);
        std::stringstream ss;
        ss << std::put_time(std::gmtime(&inTimeT), "%Y-%m-%dT%H:%M:%SZ");
        return ss.str();
    }
};

}
