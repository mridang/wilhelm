// Package v1 provides dynamic-client getters for kustomize.toolkit.fluxcd.io/v1.
//
// Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/envgen -mode crd -group kustomize.toolkit.fluxcd.io -packages github.com/fluxcd/kustomize-controller/api/v1 -assert-pkg github.com/mridang/wilhelm/assert/flux/kustomize/v1 -out zz_generated.go -package v1
package v1
