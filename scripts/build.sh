#!/usr/bin/env bash
set -euo pipefail

dnf -y update
dnf config-manager --enable crb
dnf install -y golang libnbd-devel gcc
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
if [ -d "${modules_dir}" ]; then
  for folder in "${modules_dir}"/src/*; do
    if [ -d "${folder}" ]; then
      outbin="/code/${modules_dir}/$(basename "${folder}")"
      pushd "${folder}" || return
      go build -ldflags="-s -w" -a -o "$outbin"
      popd || return
      if [ "$HAVE_UPX" = true ]; then
        echo "Compressing $outbin ..."
        upx --best --lzma --force "$outbin" || echo "UPX failed on $outbin, continuing..."
      fi
    fi
  done
else
  echo "Can't find ${modules_dir} directory, probably we're not in the collection root"
  exit 1
fi
