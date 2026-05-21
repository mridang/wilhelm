// Package v2 provides dynamic-client-based resource getters and partial
// assertion dispatchers for FluxCD's Helm controller Kinds
// (`helm.toolkit.fluxcd.io/v2`).
//
// Re-generate with:
//
//	go generate ./env/flux/helm/v2/...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/envgen -mode crd -group helm.toolkit.fluxcd.io -packages github.com/fluxcd/helm-controller/api/v2 -assert-pkg github.com/mridang/wilhelm/assert/flux/helm/v2 -out zz_generated.go -package v2
package v2
