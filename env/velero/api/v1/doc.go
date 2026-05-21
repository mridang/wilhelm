// Package v1 provides dynamic-client getters for velero.io/v1.
//
// Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/envgen -mode crd -group velero.io -packages github.com/vmware-tanzu/velero/pkg/apis/velero/v1 -assert-pkg github.com/mridang/wilhelm/assert/velero/api/v1 -out zz_generated.go -package v1
package v1
