#!/usr/bin/env bash
set -e

tag_name=vmware-migration-kit
EE_DIR="$(dirname "$0")"
cd "$EE_DIR"

ansible-builder build --tag "$tag_name"

rm -rf "$EE_DIR/context/"