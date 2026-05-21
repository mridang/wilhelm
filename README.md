# wilhelm

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Partial-matching assertions for Kubernetes resources in Go tests. Designed
for testing Helm charts, controllers, and any code that produces K8s
manifests.

> [!WARNING]
> Pre-release. The API may shift before `v1.0.0`.

## The idea

```go
import (
	"os"
	"testing"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/mridang/wilhelm/assert"
	"github.com/mridang/wilhelm/env"
)

func TestMyDeployment(t *testing.T) {
	cfg, _ := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	e, _ := env.NewEnv(cfg, "my-namespace")

	e.AssertPartial(t, "my-release", assert.DeploymentAssertion{
		Spec: assert.DeploymentSpecAssertion{
			Replicas: assert.SomePtr(int32(3)),
			Template: assert.PodTemplateSpecAssertion{
				Spec: assert.PodSpecAssertion{
					Containers: assert.Some([]assert.ContainerAssertion{
						{Name: assert.Some("app"), Image: assert.Some("nginx:1.27")},
					}),
				},
			},
		},
	})
}
```

You describe only the fields you care about. Wilhelm fetches the resource,
walks the assertion via reflection, and ignores any field left at its zero
value. Maps (labels, annotations) are subset-matched so Helm's own
metadata doesn't break the assertion.

## Repo layout

| Module                                             | Role                                                                                                                                                          |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`github.com/mridang/wilhelm`](.)                  | Core engine + generators + core K8s assertion catalogue                                                                                                       |
| [`assert/`](assert)                                | `Partial`, `Opt[T]`, `Some`, `SomePtr`, `Matching`, `Ptr`, `Assertable`, plus 500+ generated assertion structs (core K8s + Gateway API + Prometheus Operator) |
| [`env/`](env)                                      | `Env` + `NewEnv(cfg, ns)`, 138 typed resource types (`GetX` / `GetXE` per type = 276 methods), type-switching `AssertPartial` / `AssertNone`                  |
| [`internal/gen/assertgen`](internal/gen/assertgen) | Generator: emits assertion struct catalogues                                                                                                                  |
| [`internal/gen/envgen`](internal/gen/envgen)       | Generator: emits typed clientset getters or dynamic-client CRD getters (`-mode clientset` / `-mode crd`)                                                      |
| [`internal/anchors`](internal/anchors)             | Blank-import anchor for generator-scanned packages                                                                                                            |
| `assert/<family>/<v>`, `env/<family>/<v>`          | Per-CRD subpackages (separate Go modules)                                                                                                                     |

The repo is a Go multi-module monorepo. The CRD subpackages each declare
their own `go.mod`, so a downstream consumer only pulls the operator
types they actually import. A workspace file (`go.work`) keeps everything
buildable from one checkout during development.

## CRD coverage in v0.1

| Operator                  | Module                                                                                            | Group                         | Pinned upstream                                                                                  |
| ------------------------- | ------------------------------------------------------------------------------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------ |
| Flux helm-controller      | [`assert/flux/helm`](assert/flux/helm) / [`env/flux/helm`](env/flux/helm)                         | `helm.toolkit.fluxcd.io`      | `github.com/fluxcd/helm-controller/api@latest`                                                   |
| Flux source-controller    | [`assert/flux/source`](assert/flux/source) / [`env/flux/source`](env/flux/source)                 | `source.toolkit.fluxcd.io`    | `github.com/fluxcd/source-controller/api@latest`                                                 |
| Flux kustomize-controller | [`assert/flux/kustomize`](assert/flux/kustomize) / [`env/flux/kustomize`](env/flux/kustomize)     | `kustomize.toolkit.fluxcd.io` | `github.com/fluxcd/kustomize-controller/api@latest`                                              |
| cert-manager              | [`assert/cert/manager`](assert/cert/manager) / [`env/cert/manager`](env/cert/manager)             | `cert-manager.io`             | `github.com/cert-manager/cert-manager@latest`                                                    |
| Tekton Pipelines          | [`assert/tekton/pipeline`](assert/tekton/pipeline) / [`env/tekton/pipeline`](env/tekton/pipeline) | `tekton.dev`                  | `github.com/tektoncd/pipeline@latest`                                                            |
| Velero                    | [`assert/velero/api`](assert/velero/api) / [`env/velero/api`](env/velero/api)                     | `velero.io`                   | `github.com/vmware-tanzu/velero@latest`                                                          |
| Traefik                   | [`assert/traefik/io`](assert/traefik/io) / [`env/traefik/io`](env/traefik/io)                     | `traefik.io`                  | `github.com/traefik/traefik/v3@v3.2.0` (later releases require an unreplaceable local `replace`) |

### Deferred to a future release

| Operator                  | Reason                                                                                                                                                                              |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ArgoCD                    | Upstream types module references `github.com/argoproj/argo-cd/gitops-engine` at a commit hash missing a `go.mod`. Effectively unimportable.                                         |
| External Secrets Operator | Upstream types pull `sigs.k8s.io/controller-runtime` whose current release is incompatible with `k8s.io/client-go` we depend on.                                                    |
| Istio                     | `istio.io/api/networking/v1` does not follow the kubebuilder layout (no top-level Kinds with embedded `metav1.TypeMeta`/`ObjectMeta` — types live in `istio.io/client-go` instead). |
| KEDA                      | Same controller-runtime dep conflict as ESO.                                                                                                                                        |
| Crossplane                | Skipped; intent is to add via a future PR.                                                                                                                                          |

## Quick start

Importing the core packages:

```bash
go get github.com/mridang/wilhelm/assert
go get github.com/mridang/wilhelm/env
```

Importing one CRD subpackage pair (Flux Helm releases, for example):

```bash
go get github.com/mridang/wilhelm/assert/flux/helm
go get github.com/mridang/wilhelm/env/flux/helm
```

Bring your own cluster — wilhelm takes a `*rest.Config` from
`k8s.io/client-go/rest`. Wilhelm does not start clusters, install charts,
or wrap `kubectl`.

## Field semantics at a glance

| Assertion field                  | Behaviour                                                                                         |
| -------------------------------- | ------------------------------------------------------------------------------------------------- |
| Zero (`Opt[T]{}`)                | Skipped.                                                                                          |
| `Some(v)`                        | `require.Equal(actual, v)`.                                                                       |
| `SomePtr(v)`                     | Same, for pointer-typed fields.                                                                   |
| `Matching(m)`                    | Delegates to a [gomega](https://github.com/onsi/gomega) matcher. Wins over `Val` if both are set. |
| Nested `XxxAssertion` (zero)     | Skipped.                                                                                          |
| Nested `XxxAssertion` (non-zero) | Recurses.                                                                                         |
| `Opt[[]XxxAssertion]`            | Length must match; recurses element-by-element.                                                   |
| `Opt[map[K]V]` (non-empty)       | Subset match — actual may have extra keys.                                                        |
| `Opt[map[K]V]` (empty)           | Actual must also be empty.                                                                        |

## Development

```bash
devbox shell                                # or: devbox run -- <command>
make build                                  # build the root module
make test                                   # run unit tests
make lint                                   # golangci-lint
make format                                 # dprint + go fmt
make generate                               # regenerate root zz_generated.go files
make generate-all                           # regenerate every wilhelm module
make crd-list                               # list every wilhelm submodule

bash hack/scaffold-crd.sh <family> <ver> <upstream> <api-group>
                                            # scaffold a new CRD subpackage
bash hack/verify-all.sh                     # build + lint every wilhelm module
```

`devbox.json` pins Go 1.26, `golangci-lint`, `dprint`, `gotestsum`, `helm`,
and `kubectl`. CI mirrors the same toolchain.

`dprint.json` deliberately formats only Markdown, JSON, YAML and TOML —
Go formatting is handled by the standard `gofmt`/`goimports` chain
(invoked via `make format` and `golangci-lint --fix`). Keeping the two
tools disjoint avoids fight-loops on `zz_generated.go`.

## License

[Apache-2.0](LICENSE).
