#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."
docker compose -f docker/docker-compose.e2e.yml down --remove-orphans

