// Package assert provides partial-matching assertion structs for Kubernetes
// resources. Tests describe only the fields they care about via assertion
// structs; Partial walks the struct via reflection and ignores any field
// left at its zero value.
//
// Re-generate the struct catalogue (zz_generated.go) with:
//
//	go generate ./assert/...
//
//go:generate go run ../internal/gen/assertgen -out zz_generated.go -package assert
package assert
