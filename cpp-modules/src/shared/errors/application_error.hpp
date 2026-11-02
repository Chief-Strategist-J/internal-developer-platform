#pragma once

#include <exception>
#include <string>

namespace idp::shared::errors {

class ApplicationError : public std::exception {
public:
    ApplicationError(std::string message, int statusCode = 500, std::string code = "INTERNAL_SERVER_ERROR")
        : message_(std::move(message)), statusCode_(statusCode), code_(std::move(code)) {}

    [[nodiscard]] const char* what() const noexcept override {
        return message_.c_str();
    }

    [[nodiscard]] int statusCode() const noexcept {
        return statusCode_;
    }

    [[nodiscard]] const std::string& code() const noexcept {
        return code_;
    }

private:
    std::string message_;
    int statusCode_;
    std::string code_;
};

}
