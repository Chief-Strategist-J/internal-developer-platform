#include <arpa/inet.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>

#include <cstdlib>
#include <iostream>
#include <memory>
#include <sstream>
#include <string>
#include <thread>
#include <vector>

#include "api/rest/v1/handlers/health_checks_handler.hpp"
#include "api/rest/v1/routers/health_checks_router.hpp"
#include "features/health_checks/health_checks.hpp"
#include "shared/constants/system_constants.hpp"

using namespace idp;

void handleConnection(int clientFd, std::shared_ptr<api::rest::v1::routers::HealthChecksRouter> router, bool isAdmin) {
    std::vector<char> buffer(4096);
    const ssize_t bytesRead = read(clientFd, buffer.data(), buffer.size() - 1);
    if (bytesRead <= 0) {
        close(clientFd);
        return;
    }

    buffer[bytesRead] = '\0';
    std::istringstream stream(buffer.data());
    std::string method, path;
    stream >> method >> path;

    const std::string traceId = isAdmin ? "00-cpp-admin-probe-00" : "00-cpp-public-ingress-00";
    const auto routed = router->route(method, path, traceId);

    std::string response;
    if (routed.has_value()) {
        const auto& [statusCode, body] = routed.value();
        const std::string statusText = (statusCode == 200) ? "OK" : "Service Unavailable";
        std::stringstream ss;
        ss << "HTTP/1.1 " << statusCode << " " << statusText << "\r\n"
           << "Content-Type: application/json\r\n"
           << "Content-Length: " << body.length() << "\r\n"
           << "Connection: close\r\n\r\n"
           << body;
        response = ss.str();
    } else {
        if (isAdmin) {
            std::string body = "{\"success\":false,\"statusCode\":404,\"data\":null,\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"Not Found\"},\"meta\":{\"timestamp\":\"\",\"traceId\":\"00-cpp-admin-404\"}}";
            response = "HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: " + std::to_string(body.length()) + "\r\nConnection: close\r\n\r\n" + body;
        } else {
            std::string body = "{\"success\":true,\"statusCode\":200,\"data\":{\"service\":\"" + std::string(shared::constants::SERVICE_NAME) + "\",\"status\":\"RUNNING\"},\"error\":null,\"meta\":{\"timestamp\":\"\",\"traceId\":\"00-cpp-public-root\"}}";
            response = "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: " + std::to_string(body.length()) + "\r\nConnection: close\r\n\r\n" + body;
        }
    }

    write(clientFd, response.c_str(), response.length());
    close(clientFd);
}

void runListener(const std::string& host, int port, std::shared_ptr<api::rest::v1::routers::HealthChecksRouter> router, bool isAdmin) {
    const int serverFd = socket(AF_INET, SOCK_STREAM, 0);
    if (serverFd < 0) return;

    int opt = 1;
    setsockopt(serverFd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

    sockaddr_in address{};
    address.sin_family = AF_INET;
    address.sin_port = htons(port);
    inet_pton(AF_INET, host.c_str(), &address.sin_addr);

    if (bind(serverFd, reinterpret_cast<sockaddr*>(&address), sizeof(address)) < 0) {
        close(serverFd);
        return;
    }

    if (listen(serverFd, 128) < 0) {
        close(serverFd);
        return;
    }

    std::cout << (isAdmin ? "Admin" : "Public") << " server listening on http://" << host << ":" << port << std::endl;

    while (true) {
        sockaddr_in clientAddr{};
        socklen_t clientLen = sizeof(clientAddr);
        const int clientFd = accept(serverFd, reinterpret_cast<sockaddr*>(&clientAddr), &clientLen);
        if (clientFd >= 0) {
            std::thread(handleConnection, clientFd, router, isAdmin).detach();
        }
    }
}

int main() {
    int port = shared::constants::DEFAULT_PORT;
    int adminPort = shared::constants::DEFAULT_ADMIN_PORT;

    if (const char* envPort = std::getenv("PORT")) {
        port = std::atoi(envPort);
    }
    if (const char* envAdminPort = std::getenv("ADMIN_PORT")) {
        adminPort = std::atoi(envAdminPort);
    }

    auto repository = std::make_shared<features::health_checks::infrastructure::adapters::HealthChecksRepositoryAdapter>();
    auto service = std::make_shared<features::health_checks::service::HealthChecksService>(repository);
    auto handler = std::make_shared<api::rest::v1::handlers::HealthChecksHandler>(service);
    auto router = std::make_shared<api::rest::v1::routers::HealthChecksRouter>(handler);

    std::thread adminThread(runListener, shared::constants::DEFAULT_ADMIN_HOST, adminPort, router, true);
    adminThread.detach();

    runListener(shared::constants::DEFAULT_HOST, port, router, false);

    return 0;
}
