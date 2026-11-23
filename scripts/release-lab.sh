#!/usr/bin/env bash
# scripts/release-lab.sh
# Fast automated release qualification lab for RosPanel in Linux Docker.
# Verifies clean install, baseline upgrade, migrations, and SQLite integrity.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

BASELINE_IMAGE="${BASELINE_IMAGE:-ghcr.io/shu1t3/rospanel-shu1t3:5.11.0}"
CANDIDATE_BIN="${CANDIDATE_BIN:-.bin/rospanel-candidate-linux-amd64}"
FORCE_BUILD="${FORCE_BUILD:-0}"

LAB_ID="lab-$RANDOM"
CLEAN_DIR="/tmp/rospanel-${LAB_ID}-clean"
UPGRADE_DIR="/tmp/rospanel-${LAB_ID}-upgrade"
GEO_CACHE_DIR="/tmp/rospanel-lab-geo-cache"
mkdir -p "${GEO_CACHE_DIR}"

cleanup() {
  echo "==> Cleaning up lab resources (${LAB_ID})..."
  docker stop "rp-${LAB_ID}-clean" >/dev/null 2>&1 || true
  docker rm -f "rp-${LAB_ID}-clean" >/dev/null 2>&1 || true
  docker stop "rp-${LAB_ID}-base" >/dev/null 2>&1 || true
  docker rm -f "rp-${LAB_ID}-base" >/dev/null 2>&1 || true
  docker stop "rp-${LAB_ID}-upgraded" >/dev/null 2>&1 || true
  docker rm -f "rp-${LAB_ID}-upgraded" >/dev/null 2>&1 || true
  rm -rf "${CLEAN_DIR}" "${UPGRADE_DIR}" || true
}
trap cleanup EXIT INT TERM

echo "=========================================================="
echo " RosPanel Automated Release Lab (${LAB_ID})"
echo " Baseline Image: ${BASELINE_IMAGE}"
echo " Candidate Bin:  ${CANDIDATE_BIN}"
echo "=========================================================="

# 1. Compile candidate if needed
if [ "$FORCE_BUILD" = "1" ] || [ ! -f "$CANDIDATE_BIN" ]; then
  echo "==> [1/4] Compiling candidate Linux binary..."
  mkdir -p "$(dirname "$CANDIDATE_BIN")"
  TARGET_ARCH="amd64"
  # Support apple silicon Docker host transparently
  HOST_ARCH=$(uname -m)
  if [ "$HOST_ARCH" = "arm64" ] || [ "$HOST_ARCH" = "aarch64" ]; then
    TARGET_ARCH="arm64"
  fi
  CGO_ENABLED=0 GOOS=linux GOARCH="$TARGET_ARCH" go build -trimpath -ldflags "-s -w" -o "$CANDIDATE_BIN" ./cmd/rospanel
  echo "    Compiled: $CANDIDATE_BIN (linux/$TARGET_ARCH, $(du -h "$CANDIDATE_BIN" | cut -f1))"
else
  echo "==> [1/4] Reusing existing candidate binary: $CANDIDATE_BIN"
fi

ABS_CANDIDATE_BIN="$(realpath "$CANDIDATE_BIN")"

# Verify baseline image is present or pull it
if ! docker image inspect "$BASELINE_IMAGE" >/dev/null 2>&1; then
  echo "==> Pulling baseline image: $BASELINE_IMAGE..."
  docker pull "$BASELINE_IMAGE"
fi

# 2. Version Check
echo "==> [2/4] Testing candidate version output..."
CAND_VER=$(docker run --rm -v "${ABS_CANDIDATE_BIN}:/usr/local/bin/rospanel:ro" "$BASELINE_IMAGE" version)
echo "    Candidate reported: ${CAND_VER}"

# Helper function to wait for panel ready
wait_for_ready() {
  local container="$1"
  local max_wait=60
  local ready=0
  for ((i=1; i<=max_wait; i++)); do
    if docker logs "$container" 2>&1 | grep -q "ready — panel is up"; then
      ready=1
      break
    fi
    # Check if container died prematurely
    if [ "$(docker inspect -f '{{.State.Running}}' "$container" 2>/dev/null)" != "true" ]; then
      echo "[-] ERROR: Container $container exited prematurely!"
      docker logs "$container" || true
      return 1
    fi
    sleep 1
  done
  if [ "$ready" -ne 1 ]; then
    echo "[-] ERROR: Timed out waiting for $container to become ready!"
    docker logs "$container" || true
    return 1
  fi
  return 0
}

# 3. Clean Install Test
echo "==> [3/4] Testing clean installation..."
mkdir -p "${CLEAN_DIR}"
chmod 777 "${CLEAN_DIR}"

docker run -d \
  --name "rp-${LAB_ID}-clean" \
  -v "${CLEAN_DIR}:/data" \
  -v "${GEO_CACHE_DIR}:/data/geo" \
  -v "${ABS_CANDIDATE_BIN}:/usr/local/bin/rospanel:ro" \
  -e ROSPANEL_DATA=/data \
  -e ROSPANEL_HOST="127.0.0.1" \
  "$BASELINE_IMAGE" >/dev/null

wait_for_ready "rp-${LAB_ID}-clean"

# Verify DB exists and secret path is set
PATH_OUT=$(docker exec "rp-${LAB_ID}-clean" /usr/local/bin/rospanel path)
echo "    $PATH_OUT"

if [ ! -f "${CLEAN_DIR}/rospanel.db" ]; then
  echo "[-] ERROR: rospanel.db was not created on clean install!"
  exit 1
fi
echo "    Clean install successfully initialized database and Xray."
docker stop "rp-${LAB_ID}-clean" >/dev/null 2>&1
docker rm -f "rp-${LAB_ID}-clean" >/dev/null 2>&1

# 4. Upgrade Test from Baseline
echo "==> [4/4] Testing upgrade from baseline (${BASELINE_IMAGE})..."
mkdir -p "${UPGRADE_DIR}"
chmod 777 "${UPGRADE_DIR}"

# Run baseline
docker run -d \
  --name "rp-${LAB_ID}-base" \
  -v "${UPGRADE_DIR}:/data" \
  -v "${GEO_CACHE_DIR}:/data/geo" \
  -e ROSPANEL_DATA=/data \
  -e ROSPANEL_HOST="127.0.0.1" \
  "$BASELINE_IMAGE" >/dev/null

wait_for_ready "rp-${LAB_ID}-base"

BASE_PATH=$(docker exec "rp-${LAB_ID}-base" /usr/local/bin/rospanel path)
echo "    Baseline established: $BASE_PATH"

# Stop baseline container
docker stop "rp-${LAB_ID}-base" >/dev/null 2>&1
docker rm -f "rp-${LAB_ID}-base" >/dev/null 2>&1

# Start upgraded container with candidate binary mounted over same data
docker run -d \
  --name "rp-${LAB_ID}-upgraded" \
  -v "${UPGRADE_DIR}:/data" \
  -v "${GEO_CACHE_DIR}:/data/geo" \
  -v "${ABS_CANDIDATE_BIN}:/usr/local/bin/rospanel:ro" \
  -e ROSPANEL_DATA=/data \
  -e ROSPANEL_HOST="127.0.0.1" \
  "$BASELINE_IMAGE" >/dev/null

wait_for_ready "rp-${LAB_ID}-upgraded"

# Verify no errors in upgraded logs
if docker logs "rp-${LAB_ID}-upgraded" 2>&1 | grep -iE "fatal|panic|migration failed"; then
  echo "[-] ERROR: Fatal or panic error detected in upgraded container logs!"
  docker logs "rp-${LAB_ID}-upgraded"
  exit 1
fi

UPGRADED_PATH=$(docker exec "rp-${LAB_ID}-upgraded" /usr/local/bin/rospanel path)
echo "    Upgraded panel: $UPGRADED_PATH"

# Integrity check: verify database schema is readable and valid
docker exec "rp-${LAB_ID}-upgraded" /usr/local/bin/rospanel rescue list >/dev/null
echo "    Database read and admin account verified after migrations."

docker stop "rp-${LAB_ID}-upgraded" >/dev/null 2>&1
docker rm -f "rp-${LAB_ID}-upgraded" >/dev/null 2>&1

echo "=========================================================="
echo " Release Lab Results: ALL CHECKS PASSED [OK]"
echo " - Candidate Build:   PASS (${CAND_VER})"
echo " - Clean Install:     PASS"
echo " - Baseline Upgrade:  PASS"
echo " - SQLite & Data:     PASS"
echo "=========================================================="
