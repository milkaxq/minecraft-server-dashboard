#!/usr/bin/env bash
set -euo pipefail

project_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$project_dir"
if [[ ! -f .env ]]; then
  echo 'Copy .env.example to .env and configure the database passwords first.' >&2
  exit 1
fi
compose=(docker compose -f docker-compose.yml -f docker-compose.pterodactyl.yml)
# Validate without printing interpolated secrets.
"${compose[@]}" config --quiet

case "${1:-}" in
  build)
    "${compose[@]}" build panel
    ;;
  init)
    "${compose[@]}" up -d panel-database panel-cache
    "${compose[@]}" run --rm panel php artisan migrate --seed --force --no-interaction
    ;;
  admin)
    "${compose[@]}" run --rm panel php artisan p:user:make
    ;;
  panel)
    "${compose[@]}" up -d panel
    ;;
  app)
    "${compose[@]}" --profile frontend --profile pterodactyl up --build -d backend frontend worker
    ;;
  wings)
    if [[ $(uname -s) != Linux ]]; then
      echo 'The Wings stack requires native Linux Docker. Panel can run on Docker Desktop.' >&2
      exit 1
    fi
    engine_os=$(docker info --format '{{.OperatingSystem}}')
    engine_security=$(docker info --format '{{json .SecurityOptions}}')
    if [[ "$engine_os" == *"Docker Desktop"* || "$engine_security" == *"rootless"* ]]; then
      echo 'The Wings stack requires a native rootful Linux Docker Engine.' >&2
      exit 1
    fi
    if [[ ! -f .local/wings/config.yml || ! -S /var/run/docker.sock || ! -d /var/lib/pterodactyl || ! -d /tmp/pterodactyl ]]; then
      echo 'Configure .local/wings/config.yml and the Linux node paths; see docs/PTERODACTYL_V2.md.' >&2
      exit 1
    fi
    "${compose[@]}" -f docker-compose.wings.yml --profile wings up --no-deps -d wings
    ;;
  config)
    "${compose[@]}" -f docker-compose.wings.yml --profile wings config --quiet
    ;;
  down)
    # Stop the control plane; preserve volumes and Wings-managed game containers.
    "${compose[@]}" -f docker-compose.wings.yml --profile frontend --profile pterodactyl --profile wings down
    ;;
  *)
    echo 'Usage: scripts/pterodactyl.sh {build|init|admin|panel|app|wings|config|down}' >&2
    exit 1
    ;;
esac
