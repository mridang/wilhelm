// Package env provides the per-test wilhelm environment plus typed
// resource getters for every namespaced and cluster-scoped resource that
// the upstream kubernetes.Interface exposes.
//
// Re-generate the getter catalogue (zz_generated.go) with:
//
//	go generate ./env/...
//
//go:generate go run ../internal/gen/envgen -out zz_generated.go -package env
package env
