# `internal/gen/typesgen`

Extracts a self-contained copy of an upstream Go package, for upstreams that
cannot be imported as a library.

It downloads `module@version` into the module cache (no `go.mod` ever lists
it), copies the entry package whole, and from every other package of the
module copies only what the entry package reaches: the types, typed
constants and variables, and `DeepCopy*` methods. Everything lands in one
package: qualifiers that pointed into the upstream module are removed, the
reached declarations go into `zz_extracted.go`, and the run fails if two
packages contribute a type of the same name.

```bash
go run github.com/mridang/wilhelm/internal/gen/typesgen \
  -module github.com/traefik/traefik/v3 -version v3.6.25 \
  -entry pkg/provider/kubernetes/crd/traefikio/v1alpha1 \
  -out crd/v1alpha1
```

Pass `-types-only` to skip the entry package's functions and copy just its types (with `DeepCopy*` methods and typed constants), for upstreams whose functions drag in far more of the module than the types are worth. A type declared identically in two packages is kept once.

Use `-src <dir>` to read from a local checkout instead of downloading.
Bump `-version` to refresh the copy.
