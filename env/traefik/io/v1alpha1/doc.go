// Package v1alpha1 provides dynamic-client getters for traefik.io/v1alpha1.
//
// Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/envgen -mode crd -group traefik.io -packages github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1 -assert-pkg github.com/mridang/wilhelm/assert/traefik/io/v1alpha1 -out zz_generated.go -package v1alpha1
package v1alpha1
