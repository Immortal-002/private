cmake_minimum_required(VERSION 3.14)
project(telemetry_agent CXX)

set(CMAKE_CXX_STANDARD 17)

find_package(CURL REQUIRED)

add_executable(telemetry_agent
    src/main.cpp
)

target_link_libraries(telemetry_agent
    CURL::libcurl
)
