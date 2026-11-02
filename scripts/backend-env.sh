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
# Errors inside $(...) must stop the script too; a backend that failed to fetch
# would otherwise leave an empty BINDIR, and the acceptance tests skip (pass)
# when theirs is empty.
shopt -s inherit_errexit 2>/dev/null || true # bash 4.4+; the checks below do not rely on it

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
  local bin
  bin=$(find "$work/x-$recipe" -maxdepth 3 -type d -name bin | head -1)
  if [ -z "$bin" ]; then
    echo "::error::the $recipe archive has no bin directory" >&2
    return 1
  fi
  echo "$bin"
}

# bindir <recipe> [major] fetches, and stops the script if nothing came back.
# It is assigned before it is printed: a failure inside echo "X=$(...)" would
# not stop the script, because echo itself succeeds.
bindir() {
  local dir
  dir=$(fetch "$@")
  [ -n "$dir" ] && [ -d "$dir" ] || { echo "::error::no backend for $1" >&2; exit 1; }
  echo "$dir"
}

case "$module" in
  postgres)
    d=$(bindir postgres 16); echo "DOZE_POSTGRES_BINDIR=$d"
    echo "DOZE_POSTGRES_VERSION=16" ;;
  valkey)
    d=$(bindir valkey); echo "DOZE_VALKEY_BINDIR=$d" ;;
  kvrocks)
    d=$(bindir kvrocks); echo "DOZE_KVROCKS_BINDIR=$d" ;;
  ferret)
    d=$(bindir ferretdb); echo "DOZE_FERRET_BINDIR=$d"
    d=$(bindir documentdb); echo "DOZE_DOCUMENTDB_BINDIR=$d" ;;
  mariadb)
    d=$(bindir mariadb 11.4); echo "DOZE_MARIADB_BINDIR=$d"
    echo "DOZE_MARIADB_VERSION=11.4" ;;
  temporal)
    d=$(bindir temporal 1.1); echo "DOZE_TEMPORAL_BINDIR=$d"
    echo "DOZE_TEMPORAL_VERSION=1.1" ;;
  *)
    echo "::error::module '$module' has no backend mapping in scripts/backend-env.sh — add one" >&2
    exit 1 ;;
esac
