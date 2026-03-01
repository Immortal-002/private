#!/bin/bash
# Create initial admin user
# Usage: ./scripts/seed_admin.sh [username] [password]
set -e

USER=${1:-admin}
PASS=${2:-admin1234}
API=${TELEMETRY_API_URL:-http://localhost:8080}

echo "Creating admin user '${USER}' at ${API}..."

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${API}/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${USER}\",\"password\":\"${PASS}\"}")

HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | head -n -1)

if [ "$HTTP_CODE" = "201" ]; then
    echo "Admin user '${USER}' created successfully."
elif [ "$HTTP_CODE" = "409" ]; then
    echo "Admin user '${USER}' already exists."
else
    echo "Failed to create admin (HTTP ${HTTP_CODE}): ${BODY}"
    exit 1
fi

echo ""
echo "Login with:"
echo "  curl -s -X POST ${API}/api/v1/auth/login \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"username\":\"${USER}\",\"password\":\"${PASS}\"}'"
