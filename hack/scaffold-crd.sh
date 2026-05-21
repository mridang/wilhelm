#!/usr/bin/env bash
# Scaffold a new wilhelm CRD subpackage pair (assert + env).
#
# Each CRD lives in two Go modules:
#   - github.com/mridang/wilhelm/assert/<family>
#   - github.com/mridang/wilhelm/env/<family>
#
# Within each, one Go package per K8s API version (e.g. v1, v1alpha1, v2).
# The go.mod sits at the family root; packages live one directory below.
#
# Usage:
#   hack/scaffold-crd.sh <family-path> <version> <upstream-import-path> <api-group>
#
# family-path:  hierarchical path under assert/ and env/, e.g. "flux/source"
# version:      K8s API version, e.g. "v1", "v1alpha1", "v2"
# upstream:     full Go import path of the upstream types package
# api-group:    K8s API group, e.g. "source.toolkit.fluxcd.io"
#
# Example:
#   hack/scaffold-crd.sh flux/source v1 github.com/fluxcd/source-controller/api/v1 source.toolkit.fluxcd.io
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 <family-path> <version> <upstream> <api-group>" >&2
  exit 1
fi

FAMILY=$1    # e.g. flux/source
VERSION=$2   # e.g. v1
UPSTREAM=$3  # e.g. github.com/fluxcd/source-controller/api/v1
GROUP=$4     # e.g. source.toolkit.fluxcd.io

PKG_NAME=$VERSION

# Optional environment override used when the upstream's latest tag is
# broken (transitive replace, broken dep) and an older release works.
# Set WILHELM_PIN to the module version, e.g. WILHELM_PIN=v3.2.0.
PIN=${WILHELM_PIN:-latest}

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SLASHES=$(awk -F/ '{print NF-1}' <<<"$FAMILY")
# Depth from `assert/<family>` (or `env/<family>`) back to repo root.
# family has $SLASHES separators ⇒ $SLASHES+1 segments + 1 for assert/env ⇒
# total ../ count.
DOTS=$((SLASHES + 2))
DOTDOT=$(printf '../%.0s' $(seq 1 "$DOTS"))
DOTDOT=${DOTDOT%/}

scaffold_assert() {
  local family_dir="$ROOT/assert/$FAMILY"
  local pkg_dir="$family_dir/$VERSION"
  mkdir -p "$pkg_dir"

  pushd "$family_dir" >/dev/null
  if [[ ! -f go.mod ]]; then
    GOWORK=off go mod init "github.com/mridang/wilhelm/assert/$FAMILY"
  fi
  popd >/dev/null

  add_to_go_work "./assert/$FAMILY"

  cat > "$pkg_dir/anchor.go" <<EOF
package $PKG_NAME

// Blank import anchors the upstream types package in go.mod so the
// generator's packages.Load can read it.
import _ "$UPSTREAM" // generator target
EOF

  cat > "$pkg_dir/doc.go" <<EOF
// Package $PKG_NAME provides partial-match assertion structs for $GROUP/$VERSION.
//
// Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/assertgen -packages $UPSTREAM -out zz_generated.go -package $PKG_NAME -engine-pkg github.com/mridang/wilhelm/assert
package $PKG_NAME
EOF

  pushd "$family_dir" >/dev/null
  go get "$UPSTREAM@$PIN"
  go get github.com/onsi/gomega@latest github.com/stretchr/testify@latest
  go generate ./...
  go mod tidy
  go build ./...
  popd >/dev/null
}

scaffold_env() {
  local family_dir="$ROOT/env/$FAMILY"
  local pkg_dir="$family_dir/$VERSION"
  mkdir -p "$pkg_dir"

  pushd "$family_dir" >/dev/null
  if [[ ! -f go.mod ]]; then
    GOWORK=off go mod init "github.com/mridang/wilhelm/env/$FAMILY"
  fi
  popd >/dev/null

  add_to_go_work "./env/$FAMILY"

  cat > "$pkg_dir/anchor.go" <<EOF
package $PKG_NAME

// Blank import anchors the upstream types package in go.mod so the
// generator's packages.Load can read it.
import _ "$UPSTREAM" // generator target
EOF

  cat > "$pkg_dir/doc.go" <<EOF
// Package $PKG_NAME provides dynamic-client getters for $GROUP/$VERSION.
//
// Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/envgen -mode crd -group $GROUP -packages $UPSTREAM -assert-pkg github.com/mridang/wilhelm/assert/$FAMILY/$VERSION -out zz_generated.go -package $PKG_NAME
package $PKG_NAME
EOF

  pushd "$family_dir" >/dev/null
  go get "$UPSTREAM@$PIN"
  go get github.com/stretchr/testify@latest k8s.io/apimachinery@latest
  go generate ./...
  go mod tidy
  go build ./...
  popd >/dev/null
}

add_to_go_work() {
  local path=$1
  local f="$ROOT/go.work"
  if ! grep -qE "^\s*$(printf '%s\n' "$path" | sed 's/[\/&]/\\&/g')\s*$" "$f"; then
    awk -v p="$path" '/^\)/ && !done { print "\t" p; done=1 } { print }' "$f" > "$f.tmp"
    mv "$f.tmp" "$f"
  fi
}

scaffold_assert
scaffold_env

echo "OK: scaffolded $FAMILY/$VERSION (group=$GROUP, upstream=$UPSTREAM)"
