# `internal/gen/upstreamgen`

Extracts a self-contained copy of an upstream Go package, for upstreams that
cannot be imported as a library.

It downloads `module@version` into the module cache (no `go.mod` ever lists
it), copies the entry package whole, and from every other package of the
module copies only what the entry package reaches: the types, typed
constants and variables, and `DeepCopy*` methods. Imports of the upstream
module are rewritten to the generated copy.

```bash
go run github.com/mridang/wilhelm/internal/gen/upstreamgen \
  -module github.com/traefik/traefik/v3 -version v3.6.25 \
  -entry pkg/provider/kubernetes/crd/traefikio/v1alpha1 \
  -out upstream -out-import github.com/mridang/wilhelm/assert/traefik/io/upstream
```

Use `-src <dir>` to read from a local checkout instead of downloading.
Bump `-version` to refresh the copy.
