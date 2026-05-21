package assert

import "github.com/onsi/gomega/types"

// Opt wraps an optional expected value for partial assertion comparison.
// Val holds a concrete expected value; Matcher holds a gomega matcher.
// If Matcher is set it takes precedence over Val.
type Opt[T any] struct {
	Val     *T
	Matcher types.GomegaMatcher
}

// Some creates an Opt[T] that asserts equality with v.
func Some[T any](v T) Opt[T] {
	return Opt[T]{Val: &v}
}

// SomePtr creates an Opt[*T] for fields whose underlying type is a pointer.
// Use SomePtr(true) instead of Some(boolPtr(true)).
func SomePtr[T any](v T) Opt[*T] {
	return Some(&v)
}

// Matching creates an Opt[T] that uses a gomega matcher instead of equality.
func Matching[T any](m types.GomegaMatcher) Opt[T] {
	return Opt[T]{Matcher: m}
}

// Ptr returns a pointer to v. Use Ptr(int32(60)) instead of defining local
// int32Ptr closures.
func Ptr[T any](v T) *T {
	p := new(T)
	*p = v
	return p
}

// Assertable is the marker interface implemented by every generated
// assertion struct. The method is exported so CRD subpackages (and tests)
// outside the core assert package can satisfy it.
type Assertable interface {
	IsAssertable()
}
