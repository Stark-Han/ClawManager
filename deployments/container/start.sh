#!/bin/sh
set -eu

TLS_DIR="/etc/nginx/tls"
TLS_SOURCE_DIR="${TLS_SOURCE_DIR:-/var/run/clawreef-tls}"
TLS_CERT="${TLS_DIR}/tls.crt"
TLS_KEY="${TLS_DIR}/tls.key"
SOURCE_CERT="${TLS_SOURCE_DIR}/tls.crt"
SOURCE_KEY="${TLS_SOURCE_DIR}/tls.key"

mkdir -p "${TLS_DIR}"

if [ -f "${SOURCE_CERT}" ] && [ -f "${SOURCE_KEY}" ]; then
  cp "${SOURCE_CERT}" "${TLS_CERT}"
  cp "${SOURCE_KEY}" "${TLS_KEY}"
elif [ ! -f "${TLS_CERT}" ] || [ ! -f "${TLS_KEY}" ]; then
  echo "TLS certificate not found, generating a self-signed certificate for bootstrap use."
  openssl req \
    -x509 \
    -nodes \
    -days 365 \
    -newkey rsa:2048 \
    -subj "/CN=clawreef.local" \
    -keyout "${TLS_KEY}" \
    -out "${TLS_CERT}"
fi

export SERVER_ADDRESS="${SERVER_ADDRESS:-:9001}"
export SERVER_MODE="${SERVER_MODE:-release}"
export CLAWMANAGER_WORKSPACE_ARCHIVE_MAX_MIB="${CLAWMANAGER_WORKSPACE_ARCHIVE_MAX_MIB:-500}"

case "${CLAWMANAGER_WORKSPACE_ARCHIVE_MAX_MIB}" in
  ''|*[!0-9]*|0)
    echo "Invalid CLAWMANAGER_WORKSPACE_ARCHIVE_MAX_MIB=${CLAWMANAGER_WORKSPACE_ARCHIVE_MAX_MIB}; falling back to 500 MiB."
    export CLAWMANAGER_WORKSPACE_ARCHIVE_MAX_MIB="500"
    ;;
esac

sed -i "s/client_max_body_size [0-9][0-9]*m;/client_max_body_size ${CLAWMANAGER_WORKSPACE_ARCHIVE_MAX_MIB}m;/" /etc/nginx/nginx.conf

/usr/local/bin/clawreef-server &
backend_pid=$!

nginx -g 'daemon off;' &
nginx_pid=$!

shutdown() {
  kill "${backend_pid}" 2>/dev/null || true
  kill "${nginx_pid}" 2>/dev/null || true
  wait "${backend_pid}" 2>/dev/null || true
  wait "${nginx_pid}" 2>/dev/null || true
}

trap shutdown INT TERM

while kill -0 "${backend_pid}" 2>/dev/null && kill -0 "${nginx_pid}" 2>/dev/null; do
  sleep 2
done

shutdown
