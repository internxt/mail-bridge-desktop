#!/usr/bin/env bash

# Writes manifest.json and checksums.txt for the archives in RELEASE_DIR.
# The artifact names and targets are deliberately kept in sync with the public
# contract in README.md.
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <version-without-v> <release-url-base> <release-dir>" >&2
  exit 64
fi

version="$1"
release_url_base="${2%/}"
release_dir="$3"

declare -a artifacts=()
declare -a targets=(
  "windows amd64 zip mail-bridge.exe"
  "linux amd64 tar.gz mail-bridge"
  "darwin arm64 tar.gz mail-bridge"
)

cd "$release_dir"
rm -f checksums.txt manifest.json

for target in "${targets[@]}"; do
  read -r os arch archive executable <<< "$target"
  filename="mail-bridge_${version}_${os}_${arch}.${archive}"

  if [[ ! -f "$filename" ]]; then
    echo "missing release archive: $filename" >&2
    exit 1
  fi

  sha256="$(sha256sum "$filename" | awk '{print $1}')"
  printf '%s  %s\n' "$sha256" "$filename" >> checksums.txt
  artifacts+=("$(jq -cn \
    --arg os "$os" \
    --arg arch "$arch" \
    --arg archive "$archive" \
    --arg url "$release_url_base/$filename" \
    --arg sha256 "$sha256" \
    --arg executable "$executable" \
    '{os: $os, arch: $arch, archive: $archive, url: $url, sha256: $sha256, executable: $executable}')")
done

jq -n \
  --arg version "$version" \
  --argjson artifacts "[$(IFS=,; echo "${artifacts[*]}")]" \
  '{version: $version, artifacts: $artifacts}' > manifest.json
