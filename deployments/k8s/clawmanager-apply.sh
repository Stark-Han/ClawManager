#!/usr/bin/env bash
set -euo pipefail

# Edit these values before deploying, or override them with environment vars:
#   TENANT_SUFFIX=-hxc NODE_PORT=32443 APP_IMAGE=10.130.14.23:5000/clawmanager-hxc-app:team-profiles-pvfix-20260609 ./clawmanager-apply.sh
#
# TENANT_SUFFIX examples:
#   empty = clawmanager-system
#   -abc  = clawmanager-abc-system
TENANT_SUFFIX="${TENANT_SUFFIX--hxc}"
NODE_PORT="${NODE_PORT:-32443}"
APP_IMAGE="${APP_IMAGE:-10.130.14.23:5000/clawmanager-hxc-app:team-profiles-pvfix-20260609}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST="${1:-${ROOT}/clawmanager-tenant.yaml}"

case "${TENANT_SUFFIX}" in
  ""|-*) ;;
  *) TENANT_SUFFIX="-${TENANT_SUFFIX}" ;;
esac

if [[ ! -f "${MANIFEST}" ]]; then
  echo "ERROR: manifest not found: ${MANIFEST}" >&2
  echo >&2
  echo "Put clawmanager-tenant.yaml in the same directory as this script, or pass it explicitly:" >&2
  echo "  ./clawmanager-apply.sh /path/to/clawmanager-tenant.yaml" >&2
  exit 1
fi

sed "s|{TENANT_SUFFIX}|${TENANT_SUFFIX}|g;s|{NODE_PORT}|${NODE_PORT}|g;s|{APP_IMAGE}|${APP_IMAGE}|g" \
  "${MANIFEST}" | kubectl apply -f -
