# `internal/gen/assertgen`

Code generator that emits partial-match assertion structs for every
exported struct type in a list of Go packages.

## What it produces

For each exported, non-`List`, non-`Status` struct it finds, the generator
emits:

```go
// XxxAssertion is the assertion struct for Xxx.
type XxxAssertion struct {
	FieldA Opt[string]                  // scalar
	FieldB OtherAssertion               // nested Assertable
	FieldC Opt[[]OtherAssertion]        // slice of Assertable
	FieldD Opt[map[string]string]       // map (subset match)
}

// IsAssertable marks XxxAssertion as an Assertable.
func (_ XxxAssertion) IsAssertable() {}
```

Field shape:

- Type is a known struct (from one of the scanned packages) → nested
  `OtherAssertion`.
- Type is `[]KnownStruct` → `Opt[[]OtherAssertion]`.
- Anything else → `Opt[T]`.

The `Opt[T]`, `Assertable`, and `Partial` machinery is hand-written and
lives in [../../../assert](../../../assert) — the generator only emits the
struct catalogue.

## Invocation

```bash
go run github.com/mridang/wilhelm/internal/gen/assertgen \
  -out zz_generated.go \
  -package assert \
  [-packages a.b/x/v1,a.b/y/v1]
```

| Flag        | Default                   | Purpose                                                |
| ----------- | ------------------------- | ------------------------------------------------------ |
| `-out`      | _(required)_              | Output file path.                                      |
| `-package`  | `assert`                  | Package name written at the top of the generated file. |
| `-packages` | the default core-K8s list | Comma-separated Go import paths to scan.               |

Without `-packages`, the generator scans the same set zitadel-charts used
(core K8s API groups + Gateway API + Prometheus Operator). The flag lets
per-CRD subpackages target only their own upstream types so each generated
file imports a minimum set of dependencies.

## How `go generate` wires it up

[`assert/doc.go`](../../../assert/doc.go) carries the directive:

```go
//go:generate go run ../internal/gen/assertgen -out zz_generated.go -package assert
```

Each CRD subpackage gets its own [doc.go](../../../assert/doc.go)-style
file with `-packages` pointing at its target.

## Disambiguation

The same struct name (`Application`, `Cluster`, `Gateway`, ...) can exist
in multiple scanned packages. When that happens, the first occurrence
keeps the short name `XxxAssertion`; collisions get prefixed with a
package-derived word (e.g. `MetaCluster`, `ArgoApplication`).

## Determinism

Output is sorted by `pkgPath + "." + typeName` so two runs of `go generate`
in a clean checkout produce byte-identical output. CI uses this to check
that committed generated files match the upstream API.
