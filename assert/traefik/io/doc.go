// Package traefikio is the root of the Traefik traefik.io assertion module.
//
// Traefik cannot be imported as a Go library, so the upstream CRD types are
// extracted from a pinned release into the upstream directory. Re-generate
// with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/upstreamgen -module github.com/traefik/traefik/v3 -version v3.6.25 -entry pkg/provider/kubernetes/crd/traefikio/v1alpha1 -out upstream -out-import github.com/mridang/wilhelm/assert/traefik/io/upstream
package traefikio
