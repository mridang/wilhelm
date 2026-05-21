// Package v1 provides dynamic-client getters for cert-manager.io/v1.
//
// Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/envgen -mode crd -group cert-manager.io -packages github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1 -assert-pkg github.com/mridang/wilhelm/assert/cert/manager/v1 -out zz_generated.go -package v1
package v1
