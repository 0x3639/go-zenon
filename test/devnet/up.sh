#!/bin/sh
# Start the five devnet nodes on a genesis variant, from the image built by
# build-image.sh (or `make devnet-up`). Always starts from an empty chain.
#
#   test/devnet/up.sh [stock|all-off|mainnet-like|all-on|staged|fast]
#
# 'fast' runs the test build (FAST=1 test/devnet/build-image.sh) on a genesis
# with every spork and a genesis time of now.
#   test/devnet/down.sh
set -eu
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VARIANT="${1:-stock}"
cd "$ROOT"
docker compose --profile sync down -v >/dev/null 2>&1 || true
if [ "$VARIANT" = stock ]; then
  docker compose up -d --no-build pillar pillar2 pillar3 rpc observer
else
  python3 test/devnet/genesis.py >/dev/null
  DEVNET_GENESIS="$ROOT/test/devnet/genesis/$VARIANT.json"
  [ -f "$DEVNET_GENESIS" ] || { echo "unknown variant '$VARIANT'; see test/devnet/genesis.py --list" >&2; exit 1; }
  export DEVNET_GENESIS
  if [ "$VARIANT" = fast ]; then
    DEVNET_IMAGE="${DEVNET_IMAGE:-go-zenon-devnet-fast:latest}"
    export DEVNET_IMAGE
  fi
  docker compose -f docker-compose.yml -f test/devnet/compose.variant.yml up -d --no-build pillar pillar2 pillar3 rpc observer
fi
echo "devnet up on genesis variant '$VARIANT'"
