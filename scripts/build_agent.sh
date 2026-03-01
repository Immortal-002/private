#!/bin/bash
# Build the C++ telemetry agent
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
AGENT_DIR="${SCRIPT_DIR}/../agent"

echo "Building telemetry agent..."

# Check for libcurl
if ! pkg-config --exists libcurl 2>/dev/null; then
    echo "Warning: libcurl pkg-config not found. Trying to compile anyway."
fi

cd "$AGENT_DIR"

# Try cmake first, fallback to Makefile
if command -v cmake &>/dev/null; then
    mkdir -p build && cd build
    cmake ..
    make -j$(nproc 2>/dev/null || echo 2)
    echo ""
    echo "Built: ${AGENT_DIR}/build/telemetry_agent"
else
    make -j$(nproc 2>/dev/null || echo 2)
    echo ""
    echo "Built: ${AGENT_DIR}/telemetry_agent"
fi

echo ""
echo "Run with:"
echo "  TELEMETRY_API_URL=http://localhost:8080 ./telemetry_agent"
