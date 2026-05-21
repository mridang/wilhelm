# `internal/gen/envgen`

Code generator that walks `k8s.io/client-go/kubernetes.Interface` and emits
typed resource getters on `*env.Env` plus a type-switching
`AssertPartial`/`AssertNone` dispatcher.

## What it produces

For every resource discovered through the clientset (e.g.
`AppsV1Interface.Deployments(ns).Get(...)`) the generator emits:

```go
// GetDeployment fetches a Deployment by name, failing the test on error.
func (env *Env) GetDeployment(t *testing.T, name string) *appsv1.Deployment { ... }

// GetDeploymentE fetches a Deployment by name, returning the error for
// non-existence checks.
func (env *Env) GetDeploymentE(t *testing.T, name string) (*appsv1.Deployment, error) { ... }
```

Plus a single dispatcher pair:

```go
func (env *Env) AssertPartial(t *testing.T, name string, asn assert.Assertable) {
	switch a := asn.(type) {
	case assert.DeploymentAssertion:
		assert.Partial(t, env.GetDeployment(t, name), a, name)
	case *assert.DeploymentAssertion:
		assert.Partial(t, env.GetDeployment(t, name), *a, name)
	// ... 75+ more cases ...
	default:
		t.Fatalf("env.AssertPartial: no resource registered for assertion type %T", asn)
	}
}

func (env *Env) AssertNone(t *testing.T, name string, asn assert.Assertable) { ... }
```

Only resources whose Go type lives in one of the
[assertgen](../assertgen) scan packages get dispatched — the rest are
fetch-only.

## Invocation

```bash
go run github.com/mridang/wilhelm/internal/gen/envgen \
  -out zz_generated.go \
  -package env
```

| Flag       | Default      | Purpose                                                |
| ---------- | ------------ | ------------------------------------------------------ |
| `-out`     | _(required)_ | Output file path.                                      |
| `-package` | `env`        | Package name written at the top of the generated file. |

Wired through [`env/doc.go`](../../../env/doc.go):

```go
//go:generate go run ../internal/gen/envgen -out zz_generated.go -package env
```

## Name disambiguation

The same Go type name (e.g. `HorizontalPodAutoscaler`) can be reached via
multiple API group versions (`v1`, `v2`, `v1beta1`). Method names must be
unique, so the generator scores each candidate and keeps the highest-priority
one at the short name (`GetHorizontalPodAutoscaler`). The rest get prefixed
with the group method name (e.g. `GetAutoscalingV1Beta1HorizontalPodAutoscaler`).

Priority order:

1. Has an assertion struct (+1000)
2. Stable > beta > alpha (+200 / +100 / +0)
3. Higher version number wins ties (e.g. `v2` > `v1`)

## Determinism

Resources are sorted by `(GroupMethod, Name)` before rendering. Two runs
of `go generate` in a clean checkout produce byte-identical output.

## What it does not do

- **No CRD support yet.** The current generator only knows about resources
  exposed through `kubernetes.Interface`. CRD subpackages will get a
  dynamic-client-based getter generator (planned).
