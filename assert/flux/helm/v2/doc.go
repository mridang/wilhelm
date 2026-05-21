// Package v2 provides partial-match assertion structs for FluxCD's Helm
// controller Kinds (`helm.toolkit.fluxcd.io/v2`).
//
// Re-generate with:
//
//	go generate ./assert/flux/helm/v2/...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/assertgen -packages github.com/fluxcd/helm-controller/api/v2 -out zz_generated.go -package v2 -engine-pkg github.com/mridang/wilhelm/assert
package v2
