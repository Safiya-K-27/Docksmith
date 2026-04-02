#!/usr/bin/env bash
set -euo pipefail

# Creates a tiny local rootfs using host binaries and imports it into Docksmith.
# Run inside Linux/WSL2.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ROOTFS_DIR="${PROJECT_ROOT}/.docksmith-base-rootfs"

# Rebuild rootfs from scratch so previous broken imports do not persist.
rm -rf "${ROOTFS_DIR}"
mkdir -p "${ROOTFS_DIR}"/{bin,usr/bin,lib,lib64,etc,tmp,app}
chmod 1777 "${ROOTFS_DIR}/tmp"

copy_bin_and_libs() {
  local bin="$1"
  if [[ ! -x "${bin}" ]]; then
    return
  fi

  copy_path_and_resolved "${bin}"
  local resolved
  resolved="$(readlink -f "${bin}")"

  # Copy shared libraries used by this binary.
  ldd "${resolved}" | awk '{print $3}' | grep -E '^/' | while read -r lib; do
    copy_path_and_resolved "${lib}"
  done

  # Loader path appears as first field in some ldd outputs.
  ldd "${resolved}" | awk '/ld-linux|ld-musl/ {print $1}' | grep -E '^/' | while read -r ld; do
    copy_path_and_resolved "${ld}"
  done
}

copy_path_and_resolved() {
  local src="$1"
  if [[ ! -e "${src}" ]]; then
    return
  fi

  local rel="${src#/}"
  mkdir -p "${ROOTFS_DIR}/$(dirname "${rel}")"
  cp -a "${src}" "${ROOTFS_DIR}/${rel}"

  local resolved
  resolved="$(readlink -f "${src}")"
  if [[ -n "${resolved}" && "${resolved}" != "${src}" && -e "${resolved}" ]]; then
    local resolved_rel="${resolved#/}"
    mkdir -p "${ROOTFS_DIR}/$(dirname "${resolved_rel}")"
    cp -a "${resolved}" "${ROOTFS_DIR}/${resolved_rel}"

    # On merged-/usr distros, some loader links point to /lib/... while the
    # resolved file lives in /usr/lib/.... Mirror that path as well.
    if [[ "${resolved}" == /usr/lib/* ]]; then
      local lib_mirror_rel="lib/${resolved#/usr/lib/}"
      mkdir -p "${ROOTFS_DIR}/$(dirname "${lib_mirror_rel}")"
      cp -a "${resolved}" "${ROOTFS_DIR}/${lib_mirror_rel}"
    fi

    # If src is a symlink with a relative target (for example loader links),
    # also populate the symlink-target path inside rootfs.
    if [[ -L "${src}" ]]; then
      local ltarget
      ltarget="$(readlink "${src}")"
      if [[ "${ltarget}" != /* ]]; then
        local src_dir
        src_dir="$(dirname "${src}")"
        local target_abs
        target_abs="$(readlink -f "${src_dir}/${ltarget}")"
        if [[ -n "${target_abs}" && -e "${target_abs}" ]]; then
          local target_rel="${target_abs#/}"
          mkdir -p "${ROOTFS_DIR}/$(dirname "${target_rel}")"
          cp -a "${resolved}" "${ROOTFS_DIR}/${target_rel}"
        fi
      fi
    fi
  fi
}

copy_bin_and_libs /bin/sh
copy_bin_and_libs /bin/dash
copy_bin_and_libs /usr/bin/dash
copy_bin_and_libs /bin/bash
copy_bin_and_libs /usr/bin/bash
copy_bin_and_libs /bin/chmod
copy_bin_and_libs /bin/cat
copy_bin_and_libs /bin/echo
copy_bin_and_libs /usr/bin/env

cd "${PROJECT_ROOT}"
go build -o docksmith .

# Import local rootfs as base image used by the sample Docksmithfile.
./docksmith import-rootfs -t mini:latest "${ROOTFS_DIR}"

echo "Base image mini:latest imported successfully."
