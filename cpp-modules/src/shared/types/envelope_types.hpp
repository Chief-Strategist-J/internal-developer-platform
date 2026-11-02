#pragma once

#include <optional>
#include <string>

namespace idp::shared::types {

struct ApiErrorEnvelope {
    std::string code;
    std::string message;
};

struct ApiMetaEnvelope {
    std::string timestamp;
    std::string traceId;
};

template <typename T>
struct ApiResponseEnvelope {
    bool success;
    int statusCode;
    std::optional<T> data;
    std::optional<ApiErrorEnvelope> error;
    ApiMetaEnvelope meta;
};

}
