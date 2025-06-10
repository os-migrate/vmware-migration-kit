#!/usr/bin/env bash

CHANGELOG_FILE="CHANGELOG.md"
mapfile -t tag_list < <(git tag -l)

echo -ne "# Changelog\n\n" | tee "${CHANGELOG_FILE}"

for i in "${!tag_list[@]}"; do
  if [ "${i}" -lt "$((${#tag_list[@]} - 1))" ]; then
    if [ "${i}" -eq 0 ]; then
      echo -ne "## ${tag_list[${i}]}\n\n" | tee -a "${CHANGELOG_FILE}"
      echo -ne "First release\n\n" | tee -a "${CHANGELOG_FILE}"
    else
      echo -ne "## ${tag_list[$((i + 1))]}\n\n" | tee -a "${CHANGELOG_FILE}"
      git log --reverse --oneline --no-decorate "${tag_list[${i}]}..${tag_list[$((i + 1))]}" |
        cut -d ' ' -f 2- | tee -a "${CHANGELOG_FILE}"
      echo -ne "\n" | tee -a "${CHANGELOG_FILE}"
    fi
  fi
done

# delete the last empty line of the file
sed -i '${/^$/d;}' "${CHANGELOG_FILE}"
