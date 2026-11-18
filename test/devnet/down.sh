#!/bin/sh
# Stop the devnet and remove its chain state. Pass a directory to keep the logs.
#   test/devnet/down.sh [log-directory]
set -eu
cd "$(dirname "$0")/../.."
if [ -n "${1:-}" ]; then
  mkdir -p "$1"
  for n in pillar pillar2 pillar3 rpc observer syncnode; do
    docker cp "znnd-devnet-$n:/root/.znn/log/zenon.log" "$1/$n.log" 2>/dev/null || true
  done
  echo "error and critical lines per node:"; grep -cE 'lvl=(eror|crit)' "$1"/*.log || true
fi
docker compose --profile sync down -v
