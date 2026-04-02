#!/usr/bin/env bash
set -euo pipefail

# Full checklist demo. Run inside Linux/WSL2 with sudo for isolation/chroot.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SAMPLE_DIR="${PROJECT_ROOT}/examples/sample-app"

cd "${PROJECT_ROOT}"
go build -o docksmith .

# Optional one-time base import (idempotent).
"${SCRIPT_DIR}/bootstrap_base.sh"

echo "=== Cold build (expect misses) ==="
sudo ./docksmith build -t myapp:latest "${SAMPLE_DIR}"

echo "=== Warm build (expect hits) ==="
sudo ./docksmith build -t myapp:latest "${SAMPLE_DIR}"

echo "=== Edit file for partial invalidation ==="
echo "Edited at $(date -u +%s)" >> "${SAMPLE_DIR}/assets/message.txt"
sudo ./docksmith build -t myapp:latest "${SAMPLE_DIR}"

echo "=== images ==="
./docksmith images

echo "=== run default CMD ==="
sudo ./docksmith run myapp:latest

echo "=== run with env override ==="
sudo ./docksmith run -e MESSAGE=Overridden myapp:latest

echo "=== isolation check (write inside container) ==="
sudo ./docksmith run myapp:latest /bin/sh -c "echo secret > /tmp/inside-container.txt"
if [[ -f /tmp/inside-container.txt ]]; then
  echo "ERROR: host file leaked from container"
  exit 1
fi

echo "=== remove image ==="
./docksmith rmi myapp:latest

echo "Demo completed."
