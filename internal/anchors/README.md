# `internal/anchors`

Bootstrap-only package. Holds blank imports for every Go package that
wilhelm's code generators introspect, so `go.mod` keeps those modules
available to `golang.org/x/tools/go/packages.Load`.

## Why this exists

The generators ([assertgen](../gen/assertgen), [envgen](../gen/envgen)) use
`packages.Load` to read the upstream K8s and operator API packages and emit
matching assertion structs / typed getters. `packages.Load` only sees
packages that resolve through the current module graph — if no source file
imports them, `go mod tidy` removes them and the next generation produces
an empty file.

Before the generated files exist (first-ever generation, or generation in
a fresh checkout) we have a chicken-and-egg: the generated file would
anchor the deps, but the deps need to be anchored before the file can be
generated.

This package solves it with explicit blank imports.

## When to edit

Add an entry whenever you add a new package to:

- the default scan list in [`assertgen`](../gen/assertgen/main.go) (function `defaultPackages`), or
- the `assertgenPackages` set in [`envgen`](../gen/envgen/main.go).

## When not to import this

Never — `anchors` is `internal/`, has no exported symbols, and has no
runtime effect. The blank imports exist only to satisfy `go mod tidy`.
