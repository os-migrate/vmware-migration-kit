#!/usr/bin/env bash
set -euo pipefail

dnf -y update
dnf config-manager --enable crb
dnf install -y golang libnbd-devel gcc
dnf clean all

cd /code || exit
modules_dir="plugins/modules"
if [ -d "${modules_dir}" ]; then
  for folder in "${modules_dir}"/src/*; do
    if [ -d "${folder}" ]; then
      pushd "${folder}" || return
      go build -ldflags="-s -w" -a -o "/code/${modules_dir}/$(basename "${folder}")"
      popd || return
    fi
  done
else
  echo "Can't find ${modules_dir} directory, probably we're not in the collection root"
  exit 1
fi
