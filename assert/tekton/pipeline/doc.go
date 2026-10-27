// Package pipeline is the root of the Tekton tekton.dev assertion module.
//
// Tekton's go.mod only builds with a replace directive that dependents never
// see, so the upstream API types are extracted from a pinned release into the
// crd/v1 directory instead of being imported. Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/typesgen -types-only -module github.com/tektoncd/pipeline -version v1.12.0 -entry pkg/apis/pipeline/v1 -out crd/v1
package pipeline
