#pragma once

#include <cstdint>
#include <map>
#include <string>

namespace idp::features::health_checks::contracts {

struct HealthStatus {
    std::string status;
    std::string version;
    std::uint64_t uptimeSeconds;
    std::string timestamp;
};

struct ReadinessStatus {
    std::string status;
    std::map<std::string, std::string> dependencies;
    std::string timestamp;
};

}
