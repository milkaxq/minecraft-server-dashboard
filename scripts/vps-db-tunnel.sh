#!/usr/bin/env bash
set -euo pipefail

host=${BLOCKYARD_SSH_HOST:-root@169.58.54.42}
identity=${BLOCKYARD_SSH_KEY:-$HOME/.ssh/minecraft_vps}
port=${BLOCKYARD_DB_LOCAL_PORT:-55433}
if [[ ! "$port" =~ ^[0-9]+$ ]] || (( port < 1024 || port > 65535 )); then
  echo 'Use a local port between 1024 and 65535.' >&2
  exit 1
fi

# Query Docker over SSH; PostgreSQL itself has no public or host-published port.
ip=$(ssh -i "$identity" -o BatchMode=yes "$host" \
  "docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' blockyard-postgres-1")
if [[ ! "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
  echo 'Could not resolve the PostgreSQL container address.' >&2
  exit 1
fi
echo "Database tunnel listening on 127.0.0.1:$port; keep this terminal open."
exec ssh -i "$identity" -N -o BatchMode=yes -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 -o ServerAliveCountMax=3 \
  -L "127.0.0.1:$port:$ip:5432" "$host"
