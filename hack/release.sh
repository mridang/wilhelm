#!/usr/bin/env bash
# release.sh: prepare every wilhelm module for a tagged release.
#
# semantic-release invokes this from .releaserc.json's @semantic-release/exec
# in two separate lifecycle steps:
#
#   --prepare <version>  (prepareCmd)
#     Sets every `require github.com/mridang/wilhelm[...]` line to the
#     release version.
#
#   --tag <version>  (publishCmd)
#     Tags every submodule on the current commit.  This step is intentionally
#     deferred to publishCmd so the tags point at the release commit that
#     @semantic-release/git already created in the prepare phase.
#
# After `--prepare`, semantic-release commits the modified go.mod files
# (@semantic-release/git).  After that, `--tag` creates per-module Git tags
# so Go's module proxy resolves each submodule at its own tag
# (e.g. assert/flux/helm/v0.1.0).
set -euo pipefail

MODE=""
VERSION=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prepare) MODE="prepare"; shift ;;
    --tag)     MODE="tag"; shift ;;
    -*)        echo "unknown flag: $1" >&2; exit 1 ;;
    *)         VERSION="$1"; shift ;;
  esac
done

# Default to prepare for backwards compatibility (e.g. `make release-dry`).
if [[ -z "$MODE" ]]; then
  MODE="prepare"
fi

if [[ $# -ne 0 ]] || [[ -z "$VERSION" ]]; then
  echo "usage: $0 [--prepare|--tag] <version-without-leading-v>" >&2
  exit 1
fi

# ROOT is the current working directory — release.sh operates on
# whichever wilhelm checkout invoked it. This lets `make release-dry`
# point the script at a temporary copy of the tree.
ROOT=$(pwd)

# Submodule list — every directory under assert/ or env/ with its own go.mod.
mapfile -t MODULES < <(find "$ROOT/assert" "$ROOT/env" -name go.mod -print)

pin_requires() {
  local f=$1
  # Replace every wilhelm require line's pseudo-version with the release
  # version. The trailing-token match (`v[0-9][^ ]*`) preserves any
  # `// indirect` comment that follows the version. Matches both the
  # parent module and assert subpackages.
  sed -E -i.bak \
    "s|(github\.com/mridang/wilhelm[^ ]*) v[0-9][^ ]*|\1 v$VERSION|g" \
    "$f"
  rm -f "$f.bak"
}

do_prepare() {
  for mod in "${MODULES[@]}"; do
    echo "==> $mod"
    pin_requires "$mod"
  done
  echo "OK: prepared release v$VERSION across ${#MODULES[@]} submodules"
}

do_tag() {
  # Tag every module on the current commit. The publishCmd hook is invoked
  # AFTER semantic-release has already created the release commit and root
  # tag, so the submodule tags correctly point at the release SHA.
  # Skipped when invoked outside a git checkout (e.g. `make release-dry`).
  if git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1; then
    echo "==> tagging modules at v$VERSION"
    # The root tag is already created by semantic-release before publishCmd
    # runs; skip it silently if it exists so the submodule loop is not
    # short-circuited by set -e.
    git -C "$ROOT" tag "v$VERSION" 2>/dev/null || true
    for mod in "${MODULES[@]}"; do
      rel=${mod#"$ROOT/"}
      prefix=$(dirname "$rel")
      git -C "$ROOT" tag "$prefix/v$VERSION" 2>/dev/null || true
    done
    echo "OK: tagged ${#MODULES[@]} submodules + root at v$VERSION"
  else
    echo "==> skipping git tagging ($ROOT is not a git checkout)"
  fi
}

case "$MODE" in
  prepare) do_prepare ;;
  tag)     do_tag ;;
esac
