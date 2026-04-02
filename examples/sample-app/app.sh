#!/bin/sh
set -eu

echo "Sample app running in Docksmith"
echo "MESSAGE=${MESSAGE:-unset}"
if [ -f /app/built.txt ]; then
  echo "BUILD_MARKER=$(cat /app/built.txt)"
fi
if [ -f /app/assets/message.txt ]; then
  echo "ASSET=$(cat /app/assets/message.txt)"
fi
