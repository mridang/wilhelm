package assert_test

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	"github.com/mridang/wilhelm/assert"
)

// expectFailure runs fn against a fresh mock *testing.T inside a child
// goroutine, returning true if fn caused the mock to fail. Wilhelm's
// assertion engine uses testify's require package, which calls FailNow ->
// runtime.Goexit on the *testing.T. A child goroutine isolates the Goexit
// so the test process keeps running. Panics (used by the engine to flag
// developer errors like missing actual fields) are also recovered and
// reported as failures.
func expectFailure(t *testing.T, fn func(t *testing.T)) bool {
	t.Helper()
	done := make(chan struct{})
	mock := &testing.T{}
	panicked := false
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		fn(mock)
	}()
	<-done
	return mock.Failed() || panicked
}

// fixtureSpec is a small fixture struct that mirrors how the generated
// assertion structs are shaped, with one nested Assertable struct, one
// scalar, one slice, one map, and one slice-of-Assertable field.
type fixtureSpec struct {
	Replicas    *int32
	Image       string
	Labels      map[string]string
	Args        []string
	Containers  []fixtureContainer
	NestedField fixtureMeta
}

type fixtureContainer struct {
	Name  string
	Image string
}

type fixtureMeta struct {
	Name string
}

// fixtureSpecAssertion mirrors the layout of a generated assertion struct.
type fixtureSpecAssertion struct {
	Replicas    assert.Opt[*int32]
	Image       assert.Opt[string]
	Labels      assert.Opt[map[string]string]
	Args        assert.Opt[[]string]
	Containers  assert.Opt[[]fixtureContainerAssertion]
	NestedField fixtureMetaAssertion
}

func (fixtureSpecAssertion) IsAssertable() {}

type fixtureContainerAssertion struct {
	Name  assert.Opt[string]
	Image assert.Opt[string]
}

func (fixtureContainerAssertion) IsAssertable() {}

type fixtureMetaAssertion struct {
	Name assert.Opt[string]
}

func (fixtureMetaAssertion) IsAssertable() {}

// Reach into the unexported isAssertable method via the marker interface
// so the assertions are recognized by the engine.
var _ assert.Assertable = fixtureSpecAssertion{}

func TestPartial_AllZeroAssertionMatchesAnything(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{
		Image:  "nginx",
		Labels: map[string]string{"app": "demo"},
	}
	assert.Partial(t, actual, fixtureSpecAssertion{})
}

func TestPartial_ScalarMatch(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Image: "nginx:1.27"}
	assert.Partial(t, actual, fixtureSpecAssertion{
		Image: assert.Some("nginx:1.27"),
	})
}

func TestPartial_ScalarMismatch(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Image: "nginx:1.27"}
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{
			Image: assert.Some("nginx:1.99"),
		})
	})
	require.True(t, failed, "expected scalar mismatch to fail the test")
}

func TestPartial_PointerScalarWithSomePtr(t *testing.T) {
	t.Parallel()
	replicas := int32(3)
	actual := fixtureSpec{Replicas: &replicas}
	assert.Partial(t, actual, fixtureSpecAssertion{
		Replicas: assert.SomePtr(int32(3)),
	})
}

func TestPartial_MapSubsetAccepted(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Labels: map[string]string{
		"app":                          "demo",
		"helm.sh/chart":                "extra",
		"app.kubernetes.io/managed-by": "Helm",
	}}
	// Expected only mentions one label; actual has extras (Helm metadata).
	assert.Partial(t, actual, fixtureSpecAssertion{
		Labels: assert.Some(map[string]string{"app": "demo"}),
	})
}

func TestPartial_MapMissingKeyFails(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Labels: map[string]string{"app": "demo"}}
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{
			Labels: assert.Some(map[string]string{"role": "frontend"}),
		})
	})
	require.True(t, failed)
}

func TestPartial_MapEmptyExpectedRequiresEmptyActual(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Labels: map[string]string{"app": "demo"}}
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{
			Labels: assert.Some[map[string]string](nil),
		})
	})
	require.True(t, failed, "expected empty-map assertion to fail when actual has entries")
}

func TestPartial_SliceExactMatch(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Args: []string{"--port=8080"}}
	assert.Partial(t, actual, fixtureSpecAssertion{
		Args: assert.Some([]string{"--port=8080"}),
	})
}

func TestPartial_SliceOfAssertable(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Containers: []fixtureContainer{
		{Name: "main", Image: "nginx:1.27"},
		{Name: "sidecar", Image: "envoy:1.34"},
	}}
	assert.Partial(t, actual, fixtureSpecAssertion{
		Containers: assert.Some([]fixtureContainerAssertion{
			{Name: assert.Some("main")},
			{Name: assert.Some("sidecar"), Image: assert.Some("envoy:1.34")},
		}),
	})
}

func TestPartial_SliceOfAssertableLengthMismatch(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Containers: []fixtureContainer{{Name: "main"}}}
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{
			Containers: assert.Some([]fixtureContainerAssertion{
				{Name: assert.Some("main")},
				{Name: assert.Some("sidecar")},
			}),
		})
	})
	require.True(t, failed)
}

func TestPartial_NestedAssertableSkippedWhenZero(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{NestedField: fixtureMeta{Name: "anything"}}
	// NestedField in the assertion is zero — must not assert on it.
	assert.Partial(t, actual, fixtureSpecAssertion{})
}

func TestPartial_NestedAssertableMatch(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{NestedField: fixtureMeta{Name: "kube-system"}}
	assert.Partial(t, actual, fixtureSpecAssertion{
		NestedField: fixtureMetaAssertion{Name: assert.Some("kube-system")},
	})
}

func TestPartial_GomegaMatcherTakesPrecedenceOverVal(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Image: "nginx:1.27"}
	// Val would mismatch ("nginx:1.99"); Matcher must win and pass.
	assert.Partial(t, actual, fixtureSpecAssertion{
		Image: assert.Opt[string]{
			Val:     assert.Ptr("nginx:1.99"),
			Matcher: gomega.MatchRegexp(`^nginx:`),
		},
	})
}

func TestPartial_GomegaMatcherFailureFlagsTest(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Image: "nginx:1.27"}
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{
			Image: assert.Matching[string](gomega.MatchRegexp(`^redis:`)),
		})
	})
	require.True(t, failed)
}

func TestPartial_PointerToActual(t *testing.T) {
	t.Parallel()
	actual := &fixtureSpec{Image: "nginx"}
	assert.Partial(t, actual, fixtureSpecAssertion{Image: assert.Some("nginx")})
}

func TestPartial_NilActualPointerFailsLoudly(t *testing.T) {
	t.Parallel()
	var actual *fixtureSpec
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{Image: assert.Some("nginx")})
	})
	require.True(t, failed)
}

func TestPtr(t *testing.T) {
	t.Parallel()
	p := assert.Ptr(int32(42))
	require.NotNil(t, p)
	require.Equal(t, int32(42), *p)
}

func TestSomePtr(t *testing.T) {
	t.Parallel()
	o := assert.SomePtr(true)
	require.NotNil(t, o.Val)
	require.NotNil(t, *o.Val)
	require.True(t, **o.Val)
}

// deepSpec is the actual value layout for the deep-recursion tests.
type deepSpec struct {
	Outer outerLevel
}

type outerLevel struct {
	Middle middleLevel
}

type middleLevel struct {
	Inner innerLevel
}

type innerLevel struct {
	Leaf string
}

type deepSpecAssertion struct {
	Outer outerLevelAssertion
}

func (deepSpecAssertion) IsAssertable() {}

type outerLevelAssertion struct {
	Middle middleLevelAssertion
}

func (outerLevelAssertion) IsAssertable() {}

type middleLevelAssertion struct {
	Inner innerLevelAssertion
}

func (middleLevelAssertion) IsAssertable() {}

type innerLevelAssertion struct {
	Leaf assert.Opt[string]
}

func (innerLevelAssertion) IsAssertable() {}

func TestPartial_DeepNestedRecursionMatches(t *testing.T) {
	t.Parallel()
	actual := deepSpec{Outer: outerLevel{Middle: middleLevel{Inner: innerLevel{Leaf: "target"}}}}
	assert.Partial(t, actual, deepSpecAssertion{
		Outer: outerLevelAssertion{
			Middle: middleLevelAssertion{
				Inner: innerLevelAssertion{Leaf: assert.Some("target")},
			},
		},
	})
}

func TestPartial_DeepNestedRecursionFailsAtLeaf(t *testing.T) {
	t.Parallel()
	actual := deepSpec{Outer: outerLevel{Middle: middleLevel{Inner: innerLevel{Leaf: "wrong"}}}}
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, deepSpecAssertion{
			Outer: outerLevelAssertion{
				Middle: middleLevelAssertion{
					Inner: innerLevelAssertion{Leaf: assert.Some("target")},
				},
			},
		})
	})
	require.True(t, failed, "expected deep-nested mismatch to fail")
}

func TestPartial_ConcurrentExecution(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{Image: "nginx:1.27", Labels: map[string]string{"app": "demo"}}
	// Run many concurrent assertions to flush any global-state race.
	for range 64 {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			assert.Partial(t, actual, fixtureSpecAssertion{
				Image:  assert.Some("nginx:1.27"),
				Labels: assert.Some(map[string]string{"app": "demo"}),
			})
		})
	}
}

// strictSpec mirrors a generated assertion struct where the actual struct
// has extra fields the assertion doesn't reference — they must be
// ignored, not flagged.
type strictSpec struct {
	Known   string
	Unknown string
}

type strictSpecAssertion struct {
	Known assert.Opt[string]
}

func (strictSpecAssertion) IsAssertable() {}

func TestPartial_IgnoresActualFieldsNotInAssertion(t *testing.T) {
	t.Parallel()
	actual := strictSpec{Known: "hello", Unknown: "should not matter"}
	assert.Partial(t, actual, strictSpecAssertion{Known: assert.Some("hello")})
}

func TestPartial_AssertionMissingFieldOnActualPanics(t *testing.T) {
	t.Parallel()
	// Use an assertion struct that mentions a field actualMismatch doesn't
	// have. doPartial panics rather than failing the test because this is a
	// developer-side mistake. expectFailure recovers the panic and reports
	// it as a failure.
	type actualMismatch struct {
		Other string
	}
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actualMismatch{Other: "x"}, fixtureSpecAssertion{
			Image: assert.Some("nginx"),
		})
	})
	require.True(t, failed, "expected panic to be detected as a failure")
}

// envoyContainer / envoyAssertion exercises a slice-of-Assertable inside a
// slice-of-Assertable (containers each carry ports), which is the most
// recursion the generator emits in real upstream K8s types.
type envoyContainer struct {
	Name  string
	Ports []envoyPort
}

type envoyPort struct {
	Name    string
	NodePtr *int32
}

type envoyContainerAssertion struct {
	Name  assert.Opt[string]
	Ports assert.Opt[[]envoyPortAssertion]
}

func (envoyContainerAssertion) IsAssertable() {}

type envoyPortAssertion struct {
	Name    assert.Opt[string]
	NodePtr assert.Opt[*int32]
}

func (envoyPortAssertion) IsAssertable() {}

type envoyPodAssertion struct {
	Containers assert.Opt[[]envoyContainerAssertion]
}

func (envoyPodAssertion) IsAssertable() {}

type envoyPod struct {
	Containers []envoyContainer
}

// Regression: K8s controllers commonly return resources with nil slices /
// nil maps for fields the user never set. The engine must handle them
// without panicking.
func TestPartial_NilSliceInActual_ExpectedEmpty(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{} // Args == nil
	assert.Partial(t, actual, fixtureSpecAssertion{
		Args: assert.Some[[]string](nil),
	})
}

func TestPartial_NilSliceInActual_ExpectedNonEmpty(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{} // Args == nil
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{
			Args: assert.Some([]string{"--port=8080"}),
		})
	})
	require.True(t, failed)
}

func TestPartial_NilMapInActual_ExpectedEmpty(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{} // Labels == nil
	assert.Partial(t, actual, fixtureSpecAssertion{
		Labels: assert.Some[map[string]string](nil),
	})
}

func TestPartial_NilMapInActual_ExpectedNonEmpty(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{} // Labels == nil
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{
			Labels: assert.Some(map[string]string{"app": "demo"}),
		})
	})
	require.True(t, failed)
}

func TestPartial_NilSliceOfAssertableInActual(t *testing.T) {
	t.Parallel()
	actual := fixtureSpec{} // Containers == nil
	failed := expectFailure(t, func(mt *testing.T) {
		assert.Partial(mt, actual, fixtureSpecAssertion{
			Containers: assert.Some([]fixtureContainerAssertion{
				{Name: assert.Some("main")},
			}),
		})
	})
	require.True(t, failed)
}

func TestPartial_SliceOfAssertableNested(t *testing.T) {
	t.Parallel()
	node := int32(30080)
	actual := envoyPod{Containers: []envoyContainer{{
		Name: "envoy",
		Ports: []envoyPort{
			{Name: "http", NodePtr: &node},
			{Name: "metrics"},
		},
	}}}
	assert.Partial(t, actual, envoyPodAssertion{
		Containers: assert.Some([]envoyContainerAssertion{{
			Name: assert.Some("envoy"),
			Ports: assert.Some([]envoyPortAssertion{
				{Name: assert.Some("http"), NodePtr: assert.SomePtr(int32(30080))},
				{Name: assert.Some("metrics")},
			}),
		}}),
	})
}
