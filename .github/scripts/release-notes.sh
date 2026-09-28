#!/bin/sh
# Prints the CHANGELOG.md section for a version, such as v0.1.0 or 0.1.0.
# Exits 1 when the section is missing or empty, so a release can't go out
# without notes.
set -eu
version="${1#v}"
notes=$(awk -v v="$version" '
  /^## \[/ { p = index($0, "[" v "]") > 0; next }
  p
' "${2:-CHANGELOG.md}" | sed -e '/./,$!d')
if [ -z "$notes" ]; then
  echo "CHANGELOG.md has no section for $version" >&2
  exit 1
fi
printf '%s\n' "$notes"
