#!/usr/bin/env bash
set -euo pipefail

unformatted=$(go tool goimports -local minecraft-hosting -l cmd internal migrations)
if [[ -n "$unformatted" ]]; then
  printf 'Go files need formatting:\n%s\nRun make fmt-go to fix them.\n' "$unformatted" >&2
  exit 1
fi
