#!/bin/bash

# Get token first
echo "Getting auth token..."
TOKEN=$(curl -s -X POST http://localhost:9001/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"test123"}' | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
    echo "Failed to get token"
    exit 1
fi

echo "Token: $TOKEN"
echo ""

# Force sync instance 0
echo "Force syncing instance 0..."
curl -s -X POST http://localhost:9001/api/v1/instances/0/sync \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" | jq .
