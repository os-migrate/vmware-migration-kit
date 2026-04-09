#!/usr/bin/env bash
set -euo pipefail

dnf -y update
dnf config-manager --enable crb
dnf install -y golang libnbd-devel gcc epel-release
if ! dnf install -y upx >/dev/null 2>&1; then
  echo "UPX not available in dnf — skipping UPX installation."
fi
dnf clean all

# Check if UPX is available for later use
if command -v upx >/dev/null 2>&1; then
  HAVE_UPX=true
  echo "UPX found — binaries will be compressed."
else
  HAVE_UPX=false
  echo "UPX not found — skipping compression."
fi

cd /code || exit
modules_dir="plugins/modules"
if [ ! -d "${modules_dir}" ]; then
  echo "Can't find ${modules_dir} directory, probably we're not in the collection root"
  exit 1
fi

# Single pass over all src/ subdirectories:
#   - package main  -> compile as standalone binary
#   - library package -> create a symlink to module_dispatcher so Ansible
#                        wrappers keep working without maintaining a list
for folder in "${modules_dir}"/src/*; do
  [ -d "${folder}" ] || continue

  module_name="$(basename "${folder}")"
  pushd "${folder}" || exit
  pkg_name="$(go list -f '{{.Name}}' .)"
  popd || exit

  if [ "${pkg_name}" = "main" ]; then
    outbin="/code/${modules_dir}/${module_name}"
    echo "Building ${module_name} ..."
    pushd "${folder}" || exit
    go build -trimpath -ldflags="-s -w -buildid=" -o "$outbin"
    popd || exit
    if [ "$HAVE_UPX" = true ]; then
      echo "Compressing $outbin ..."
      upx --best --lzma --force "$outbin" || echo "UPX failed on $outbin, continuing..."
    fi
  else
    echo "Symlinking ${module_name} -> module_dispatcher"
    ln -sf module_dispatcher "/code/${modules_dir}/${module_name}"
  fi
done
