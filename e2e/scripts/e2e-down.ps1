Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Push-Location (Join-Path $PSScriptRoot "..")
try {
  docker compose -f docker/docker-compose.e2e.yml down --remove-orphans
} finally {
  Pop-Location
}

