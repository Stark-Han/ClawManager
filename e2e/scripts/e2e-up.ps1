Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Push-Location (Join-Path $PSScriptRoot "..")
try {
  docker compose -f docker/docker-compose.e2e.yml up -d mysql
} finally {
  Pop-Location
}

