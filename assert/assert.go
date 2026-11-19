package assert

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/onsi/gomega/types"
	"github.com/stretchr/testify/require"
)

// assertableType is the reflect.Type of the Assertable marker interface,
// computed once at package init.
//
//nolint:gochecknoglobals // single immutable reflect.Type cached at init
var assertableType = reflect.TypeFor[Assertable]()

// Partial walks the assertion struct via reflection and compares set fields
// against actual. Fields left at their zero value are skipped.
//
// actual may be a struct or a pointer to a struct. assertion must implement
// Assertable. msgAndArgs is forwarded to the underlying require calls.
func Partial[T Assertable](
	t *testing.T,
	actual any,
	assertion T,
	msgAndArgs ...any,
) {
	t.Helper()
	doPartial(t, reflect.ValueOf(actual), reflect.ValueOf(assertion), msgAndArgs)
}

func doPartial(t *testing.T, actualVal, assertionVal reflect.Value, msgAndArgs []any) {
	t.Helper()

	actualVal, ok := derefPtr(t, actualVal, "actual is nil but assertion has fields to check", msgAndArgs)
	if !ok {
		return
	}
	assertionVal, ok = derefPtr(t, assertionVal, "assertion is nil but has fields to check", msgAndArgs)
	if !ok {
		return
	}

	if assertionVal.Kind() != reflect.Struct {
		panic(fmt.Sprintf(
			"AssertPartial: assertion must be a struct or pointer to struct, got %v",
			assertionVal.Kind(),
		))
	}

	assertionType := assertionVal.Type()
	for i := range assertionType.NumField() {
		fieldInfo := assertionType.Field(i)
		fieldVal := assertionVal.Field(i)

		actualField := actualVal.FieldByName(fieldInfo.Name)
		if !actualField.IsValid() {
			panic(fmt.Sprintf(
				"AssertPartial: actual struct %v does not have field %q",
				actualVal.Type(), fieldInfo.Name,
			))
		}
		assertField(t, fieldInfo.Name, actualField, fieldVal, msgAndArgs)
	}
}

func derefPtr(
	t *testing.T,
	v reflect.Value,
	failMsg string,
	msgAndArgs []any,
) (reflect.Value, bool) {
	t.Helper()
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			require.Fail(t, failMsg, msgAndArgs...)
			return v, false
		}
		v = v.Elem()
	}
	return v, true
}

func assertField(
	t *testing.T,
	name string,
	actualField, fieldVal reflect.Value,
	msgAndArgs []any,
) {
	t.Helper()

	if fieldVal.Type().Implements(assertableType) {
		if fieldVal.IsZero() {
			return
		}
		doPartial(t, actualField, fieldVal, msgAndArgs)
		return
	}

	if tryMatcher(t, name, actualField, fieldVal, msgAndArgs) {
		return
	}

	valField := fieldVal.FieldByName("Val")
	if !valField.IsValid() {
		panic(fmt.Sprintf(
			"AssertPartial: field %q of type %v is neither Assertable nor Opt[T]",
			name, fieldVal.Type(),
		))
	}
	if valField.IsNil() {
		return
	}

	compareVal(t, name, actualField, valField.Elem(), msgAndArgs)
}

// tryMatcher applies the gomega matcher in fieldVal if one is set. It
// returns true if a matcher was applied (in which case the caller should
// skip exact-value comparison).
func tryMatcher(
	t *testing.T,
	name string,
	actualField, fieldVal reflect.Value,
	msgAndArgs []any,
) bool {
	t.Helper()
	matcherField := fieldVal.FieldByName("Matcher")
	if !matcherField.IsValid() || matcherField.IsNil() {
		return false
	}
	matcher, ok := matcherField.Interface().(types.GomegaMatcher)
	if !ok {
		panic(fmt.Sprintf(
			"AssertPartial: field %q Matcher does not implement GomegaMatcher",
			name,
		))
	}
	success, err := matcher.Match(actualField.Interface())
	require.NoError(
		t, err,
		append([]any{fmt.Sprintf("field %q: matcher error", name)}, msgAndArgs...)...,
	)
	if !success {
		require.Fail(
			t,
			matcher.FailureMessage(actualField.Interface()),
			append([]any{fmt.Sprintf("field %q", name)}, msgAndArgs...)...,
		)
	}
	return true
}

func compareVal(
	t *testing.T,
	name string,
	actualField, expectedVal reflect.Value,
	msgAndArgs []any,
) {
	t.Helper()

	if expectedVal.Kind() == reflect.Slice &&
		expectedVal.Type().Elem().Implements(assertableType) {
		compareAssertableSlice(t, name, actualField, expectedVal, msgAndArgs)
		return
	}

	if expectedVal.Kind() == reflect.Map {
		compareMapSubset(t, name, actualField, expectedVal, msgAndArgs)
		return
	}

	require.Equal(
		t, expectedVal.Interface(), actualField.Interface(),
		append([]any{fmt.Sprintf("field %q mismatch", name)}, msgAndArgs...)...,
	)
}

func compareAssertableSlice(
	t *testing.T,
	name string,
	actualField, expectedVal reflect.Value,
	msgAndArgs []any,
) {
	t.Helper()
	require.Equal(
		t, expectedVal.Len(), actualField.Len(),
		append([]any{fmt.Sprintf("field %q: slice length mismatch", name)}, msgAndArgs...)...,
	)
	for j := range expectedVal.Len() {
		elemMsg := append(
			[]any{fmt.Sprintf("field %q[%d]", name, j)},
			msgAndArgs...,
		)
		doPartial(t, actualField.Index(j), expectedVal.Index(j), elemMsg)
	}
}

// compareMapSubset verifies all expected entries exist in actual; actual may
// have extra keys (e.g. Helm metadata annotations on labels/annotations
// maps). An empty expected map means "assert no entries"; a nil actual is
// treated as empty.
func compareMapSubset(
	t *testing.T,
	name string,
	actualField, expectedVal reflect.Value,
	msgAndArgs []any,
) {
	t.Helper()
	if expectedVal.Len() == 0 {
		if actualField.Kind() == reflect.Map && actualField.Len() > 0 {
			require.Fail(
				t,
				fmt.Sprintf(
					"field %q: expected empty map but actual has %d entries",
					name, actualField.Len(),
				),
				msgAndArgs...,
			)
		}
		return
	}
	for _, k := range expectedVal.MapKeys() {
		actualEntry := actualField.MapIndex(k)
		require.True(
			t, actualEntry.IsValid(),
			append([]any{fmt.Sprintf(
				"field %q: expected key %v not found in actual map",
				name, k.Interface(),
			)}, msgAndArgs...)...,
		)
		require.Equal(
			t, expectedVal.MapIndex(k).Interface(), actualEntry.Interface(),
			append([]any{fmt.Sprintf(
				"field %q[%v] mismatch", name, k.Interface(),
			)}, msgAndArgs...)...,
		)
	}
}
