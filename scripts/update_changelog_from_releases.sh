#!/usr/bin/env bash

CHANGELOG_FILE="CHANGELOG.md"
mapfile -t tag_list < <(git tag -l)

echo -ne "# Changelog\n\n" | tee "${CHANGELOG_FILE}"

for i in "${!tag_list[@]}"; do
  gh release view --json tagName,body "${tag_list[i]}" | jq -r '"","## \(.tagName)","",.body' | tee -a "${CHANGELOG_FILE}"
done

echo "Be aware that you need to fix the markdown linting error!"
