// Package v1 provides dynamic-client getters for tekton.dev/v1.
//
// Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/envgen -mode crd -group tekton.dev -packages github.com/tektoncd/pipeline/pkg/apis/pipeline/v1 -assert-pkg github.com/mridang/wilhelm/assert/tekton/pipeline/v1 -out zz_generated.go -package v1
package v1
