cmake_minimum_required(VERSION 3.20)
project(internal-developer-platform-cpp CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include_directories(src)

file(GLOB_RECURSE SOURCES "src/*.cpp")

add_executable(idp-cpp-service ${SOURCES})

find_package(Threads REQUIRED)
target_link_libraries(idp-cpp-service PRIVATE Threads::Threads)
