// Package v1 provides partial-match assertion structs for cert-manager.io/v1.
//
// Re-generate with:
//
//	go generate ./...
//
//go:generate go run github.com/mridang/wilhelm/internal/gen/assertgen -packages github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1 -out zz_generated.go -package v1 -engine-pkg github.com/mridang/wilhelm/assert
package v1
