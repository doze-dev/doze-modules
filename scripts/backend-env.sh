#!/usr/bin/env bash
# backend-env.sh <module> [triple]
#
# Fetches the prebuilt backend(s) a module's acceptance tests need from the
# doze-binaries releases, and prints the environment those tests read
# (DOZE_<ENGINE>_BINDIR, and DOZE_<ENGINE>_VERSION where a major is pinned), one
# KEY=VALUE per line. CI appends the output to $GITHUB_ENV:
#
#   scripts/backend-env.sh postgres >> "$GITHUB_ENV"
#
# These are the same artifacts users download, so acceptance tests what users
# run. Building backends from source is doze-binaries' job, not this repo's.
set -euo pipefail

module=${1:?usage: backend-env.sh <module> [triple]}
triple=${2:-x86_64-unknown-linux-gnu}
work=${RUNNER_TEMP:-/tmp}/doze-backends
mkdir -p "$work"

# fetch <recipe> [major] prints the bin directory of the unpacked backend.
fetch() {
  local recipe=$1 major=${2:-} idx="$work/$1-index.yaml" full url
  if ! curl -sfL --retry 3 --retry-all-errors \
      "https://github.com/doze-dev/doze-binaries/releases/download/$recipe/index.yaml" -o "$idx"; then
    echo "::error::no prebuilt $recipe backend in doze-binaries — users cannot boot this module; publish the backend first" >&2
    return 1
  fi
  if [ -n "$major" ]; then
    full=$(yq ".engines.$recipe.versions.\"$major\"" "$idx")
  else
    full=$(yq ".engines.$recipe.versions | to_entries | .[-1].value" "$idx")
  fi
  url=$(yq ".engines.$recipe.artifacts.\"$full\".\"$triple\".url" "$idx")
  if [ "$url" = "null" ]; then
    echo "::error::no $triple artifact for $recipe $full" >&2
    return 1
  fi
  mkdir -p "$work/x-$recipe"
  curl -sfL --retry 3 --retry-all-errors "$url" | tar -xz -C "$work/x-$recipe"
  find "$work/x-$recipe" -maxdepth 3 -type d -name bin | head -1
}

case "$module" in
  postgres)
    echo "DOZE_POSTGRES_BINDIR=$(fetch postgres 16)"
    echo "DOZE_POSTGRES_VERSION=16" ;;
  valkey)
    echo "DOZE_VALKEY_BINDIR=$(fetch valkey)" ;;
  kvrocks)
    echo "DOZE_KVROCKS_BINDIR=$(fetch kvrocks)" ;;
  ferret)
    echo "DOZE_FERRET_BINDIR=$(fetch ferretdb)"
    echo "DOZE_DOCUMENTDB_BINDIR=$(fetch documentdb)" ;;
  mariadb)
    echo "DOZE_MARIADB_BINDIR=$(fetch mariadb 11.4)"
    echo "DOZE_MARIADB_VERSION=11.4" ;;
  temporal)
    echo "DOZE_TEMPORAL_BINDIR=$(fetch temporal 1.1)"
    echo "DOZE_TEMPORAL_VERSION=1.1" ;;
  *)
    echo "::error::module '$module' has no backend mapping in scripts/backend-env.sh — add one" >&2
    exit 1 ;;
esac
