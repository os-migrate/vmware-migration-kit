#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   ./release.sh --version 2.0.8 --changelog "Some description"

VERSION=""
CHANGELOG=""

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --changelog)
      CHANGELOG="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1"
      exit 1
      ;;
  esac
done

if [[ -z "$VERSION" ]]; then
  echo "Error: --version is required"
  exit 1
fi

if [[ -z "$CHANGELOG" ]]; then
  echo "Warning: --changelog is empty"
fi

echo "Bumping release to version $VERSION"
echo "Changelog entry: $CHANGELOG"

########################################
# Update galaxy.yml
########################################
if [[ -f galaxy.yml ]]; then
  echo "Updating galaxy.yml..."
  sed -i'' -e "s/^version: .*/version: ${VERSION}/" galaxy.yml
else
  echo "galaxy.yml not found!"
  exit 1
fi

########################################
# Build the collection tarball
########################################
echo "Building collection tarball..."
TARBALL=$(ansible-galaxy collection build --force | awk '/Created collection tarball/ {print $NF}')
echo "Built tarball: $TARBALL"

########################################
# Update aee/requirements.yml
########################################
if [[ -f aee/requirements.yml ]]; then
  echo "Updating aee/requirements.yml..."
  sed -E -i'' -e "s|(os_migrate-vmware_migration_kit-)[0-9]+\.[0-9]+\.[0-9]+(\.tar\.gz)|\1${VERSION}\2|" aee/requirements.yml
else
  echo "aee/requirements.yml not found!"
  exit 1
fi

########################################
# Update aee/execution-environment.yml
########################################
if [[ -f aee/execution-environment.yml ]]; then
  echo "Updating aee/execution-environment.yml..."
  sed -E -i'' -e "s|(os_migrate-vmware_migration_kit-)[0-9]+\.[0-9]+\.[0-9]+(\.tar\.gz)|\1${VERSION}\2|" aee/execution-environment.yml
else
  echo "aee/execution-environment.yml not found!"
  exit 1
fi

########################################
# Update CHANGELOG.md (append at end)
########################################
if [[ -f CHANGELOG.md ]]; then
  echo "Updating CHANGELOG.md..."
  {
    echo ""
    echo "## v${VERSION}"
    echo ""
    echo "- ${CHANGELOG}"
  } >> CHANGELOG.md
else
  echo "CHANGELOG.md not found!"
  exit 1
fi

echo "Release bump to $VERSION complete!"
echo "Generated tarball: $TARBALL"
echo "You can publish the collection with:"
echo "  ansible-galaxy collection publish <full path> --api-key <your-api-key>"
exit 0
